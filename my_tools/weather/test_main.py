"""天气接口使用固定响应，测试不会访问网络。"""

import io
import json
import unittest
from contextlib import redirect_stderr, redirect_stdout
from unittest.mock import patch
from urllib.parse import parse_qs, urlparse

import main as weather


class WeatherTest(unittest.TestCase):
    """覆盖标准输入输出协议、地点解析和网络失败。"""

    def test_success(self):
        """查询成功时输出一个完整 JSON，并保留地点、单位和数据时间。"""
        locations = {
            "results": [{
                "name": "成都",
                "admin1": "四川",
                "country": "中国",
                "latitude": 30.67,
                "longitude": 104.06,
            }]
        }
        forecast = {
            "current": {
                "temperature_2m": 22.5,
                "relative_humidity_2m": 60,
                "wind_speed_10m": 8.2,
                "weather_code": 3,
                "time": "2026-11-04T12:00",
            },
            "current_units": {
                "temperature_2m": "°C",
                "relative_humidity_2m": "%",
                "wind_speed_10m": "km/h",
            },
            "timezone": "Asia/Shanghai",
        }
        stdout = io.StringIO()
        stderr = io.StringIO()

        with (
            patch.object(weather, "fetch_json", side_effect=[locations, forecast]) as fetch,
            patch("sys.stdin", io.StringIO('{"city":" 成都 "}')),
            redirect_stdout(stdout),
            redirect_stderr(stderr),
        ):
            exit_code = weather.main()

        result = json.loads(stdout.getvalue())
        self.assertEqual(exit_code, 0)
        self.assertEqual(stderr.getvalue(), "")
        self.assertEqual(result["city"], "成都")
        self.assertEqual(result["temperature"], "22.5 °C")
        self.assertEqual(result["condition"], "阴")
        self.assertEqual(result["observed_at"], "2026-11-04T12:00")

        first_query = parse_qs(urlparse(fetch.call_args_list[0].args[0]).query)
        second_query = parse_qs(urlparse(fetch.call_args_list[1].args[0]).query)
        self.assertEqual(first_query["name"], ["成都"])
        self.assertEqual(second_query["latitude"], ["30.67"])

    def test_invalid_input(self):
        """输入无效时直接失败，不发出网络请求。"""
        for source in ['{}', '[]', '{"city":42}', '{"city":" "}', '{']:
            with self.subTest(source=source):
                stdout = io.StringIO()
                stderr = io.StringIO()

                with (
                    patch.object(weather, "fetch_json") as fetch,
                    patch("sys.stdin", io.StringIO(source)),
                    redirect_stdout(stdout),
                    redirect_stderr(stderr),
                ):
                    self.assertEqual(weather.main(), 1)

                fetch.assert_not_called()
                self.assertEqual(stdout.getvalue(), "")
                self.assertIn("查询天气失败", stderr.getvalue())

    def test_missing_city(self):
        """未知城市不能伪造天气结果。"""
        with patch.object(weather, "fetch_json", return_value={}):
            with self.assertRaisesRegex(ValueError, "未找到城市"):
                weather.current_weather("不存在的城市")

    def test_network_failure(self):
        """联网失败时只在 stderr 写入原因并返回非零退出码。"""
        stdout = io.StringIO()
        stderr = io.StringIO()

        with (
            patch.object(weather, "fetch_json", side_effect=OSError("连接超时")),
            patch("sys.stdin", io.StringIO('{"city":"成都"}')),
            redirect_stdout(stdout),
            redirect_stderr(stderr),
        ):
            self.assertEqual(weather.main(), 1)

        self.assertEqual(stdout.getvalue(), "")
        self.assertIn("连接超时", stderr.getvalue())


if __name__ == "__main__":
    unittest.main()
