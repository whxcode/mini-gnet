"""城市天气示例：stdin 接收参数，stdout 返回业务 JSON，stderr 报告失败。"""

import json
import sys
from urllib.parse import urlencode
from urllib.request import Request, urlopen


def fetch_json(url):
    """请求公开天气接口；超时或服务错误直接交给调用方处理。"""
    request = Request(url, headers={"User-Agent": "mini-gnet-weather/1.0"})

    with urlopen(request, timeout=10) as response:
        data = response.read(1024 * 1024 + 1)

    if len(data) > 1024 * 1024:
        raise ValueError("天气接口响应超过 1 MiB")

    result = json.loads(data)

    if result.get("error"):
        raise ValueError(result.get("reason", "天气服务返回错误"))

    return result


def weather_description(code):
    """将 Open-Meteo 使用的 WMO 天气代码转换成简单中文描述。"""
    descriptions = {
        0: "晴",
        1: "大部晴朗",
        2: "局部多云",
        3: "阴",
        45: "雾",
        48: "雾凇",
        51: "小毛毛雨",
        53: "中毛毛雨",
        55: "大毛毛雨",
        56: "轻度冻毛毛雨",
        57: "强冻毛毛雨",
        61: "小雨",
        63: "中雨",
        65: "大雨",
        66: "轻度冻雨",
        67: "强冻雨",
        71: "小雪",
        73: "中雪",
        75: "大雪",
        77: "雪粒",
        80: "小阵雨",
        81: "中阵雨",
        82: "强阵雨",
        85: "小阵雪",
        86: "强阵雪",
        95: "雷暴",
        96: "雷暴伴小冰雹",
        99: "雷暴伴大冰雹",
    }

    return descriptions.get(code, f"未知天气代码 {code}")


def current_weather(city):
    """先将城市转换为坐标，再查询该坐标的当前天气。"""
    query = urlencode({
        "name": city,
        "count": 1,
        "language": "zh",
        "format": "json",
    })
    locations = fetch_json(
        "https://geocoding-api.open-meteo.com/v1/search?" + query
    )

    if not locations.get("results"):
        raise ValueError(f"未找到城市：{city}，可以尝试城市英文名")

    location = locations["results"][0]
    query = urlencode({
        "latitude": location["latitude"],
        "longitude": location["longitude"],
        "current": "temperature_2m,relative_humidity_2m,weather_code,wind_speed_10m",
        "timezone": "auto",
    })
    forecast = fetch_json("https://api.open-meteo.com/v1/forecast?" + query)
    current = forecast["current"]
    units = forecast["current_units"]

    return {
        "city": location["name"],
        "region": location.get("admin1", ""),
        "country": location.get("country", ""),
        "temperature": f"{current['temperature_2m']} {units['temperature_2m']}",
        "humidity": f"{current['relative_humidity_2m']} {units['relative_humidity_2m']}",
        "wind_speed": f"{current['wind_speed_10m']} {units['wind_speed_10m']}",
        "condition": weather_description(current["weather_code"]),
        "observed_at": current["time"],
        "timezone": forecast["timezone"],
        "source": "Open-Meteo (https://open-meteo.com/)",
    }


def main():
    """读取一次调用并返回一次结果，日志不会混入 stdout。"""
    try:
        arguments = json.load(sys.stdin)

        if not isinstance(arguments, dict):
            raise ValueError("输入必须是 JSON 对象")

        city = arguments.get("city")

        if not isinstance(city, str) or not city.strip():
            raise ValueError("city 必须是非空城市名称")

        result = current_weather(city.strip())
        print(json.dumps(result, ensure_ascii=False, allow_nan=False))

        return 0

    except (ValueError, KeyError, TypeError, AttributeError, OSError) as error:
        print(f"查询天气失败：{error}", file=sys.stderr)

        return 1


if __name__ == "__main__":
    sys.exit(main())
