#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""天气查询小工具

用法:
    python main.py
    然后输入城市名（如 shanghai / 上海 / tokyo / new york）
    输入 q 或直接回车退出

数据来源: Open-Meteo 公开免费接口, 无需 API Key。
依赖: 仅 Python 3.7+ 标准库。
"""

import json
import sys
import urllib.error
import urllib.parse
import urllib.request

GEOCODE_URL = "https://geocoding-api.open-meteo.com/v1/search"
FORECAST_URL = "https://api.open-meteo.com/v1/forecast"
TIMEOUT = 10  # 秒

# WMO 天气代码 -> 中文描述
WMO_CODES = {
    0: "晴",
    1: "大部晴朗",
    2: "多云",
    3: "阴",
    45: "雾",
    48: "冻雾",
    51: "小毛毛雨",
    53: "中等毛毛雨",
    55: "浓毛毛雨",
    56: "轻微冻毛毛雨",
    57: "强冻毛毛雨",
    61: "小雨",
    63: "中雨",
    65: "大雨",
    66: "轻微冻雨",
    67: "强冻雨",
    71: "小雪",
    73: "中雪",
    75: "大雪",
    77: "雪粒",
    80: "小阵雨",
    81: "中等阵雨",
    82: "强阵雨",
    85: "小阵雪",
    86: "大阵雪",
    95: "雷阵雨",
    96: "雷阵雨伴小冰雹",
    99: "雷阵雨伴大冰雹",
}


def http_get_json(url, params):
    """拼接查询参数并请求 JSON 接口。"""
    full_url = url + "?" + urllib.parse.urlencode(params)
    req = urllib.request.Request(
        full_url,
        headers={"User-Agent": "python-weather-cli/1.0"},
    )
    with urllib.request.urlopen(req, timeout=TIMEOUT) as resp:
        return json.loads(resp.read().decode("utf-8"))


def search_city(name):
    """按城市名查询经纬度信息, 找不到返回 None。"""
    data = http_get_json(
        GEOCODE_URL,
        {"name": name, "count": 1, "language": "zh", "format": "json"},
    )
    results = data.get("results") or []
    return results[0] if results else None


def fetch_weather(latitude, longitude):
    """查询当前天气。"""
    return http_get_json(
        FORECAST_URL,
        {
            "latitude": latitude,
            "longitude": longitude,
            "current": ",".join(
                [
                    "temperature_2m",
                    "relative_humidity_2m",
                    "apparent_temperature",
                    "weather_code",
                    "wind_speed_10m",
                    "precipitation",
                ]
            ),
            "timezone": "auto",
        },
    )


def describe_location(place):
    """拼出城市全称, 如: 上海（上海市, 中国）。"""
    name = place.get("name", "未知城市")
    parts = [p for p in (place.get("admin1"), place.get("country")) if p]
    parts = [p for p in parts if p != name]
    if parts:
        return "{}（{}）".format(name, ", ".join(parts))
    return name


def print_weather(place, data):
    current = data.get("current") or {}
    code = current.get("weather_code")
    desc = WMO_CODES.get(code, "未知天气({})".format(code))

    print()
    print("城市:", describe_location(place))
    print("时间: {} ({})".format(current.get("time", "-"), data.get("timezone", "-")))
    print("天气:", desc)
    print(
        "温度: {} °C（体感 {} °C）".format(
            current.get("temperature_2m", "-"),
            current.get("apparent_temperature", "-"),
        )
    )
    print("湿度: {}%".format(current.get("relative_humidity_2m", "-")))
    print("风速: {} km/h".format(current.get("wind_speed_10m", "-")))
    print("降水: {} mm".format(current.get("precipitation", "-")))
    print()


def query(city_input):
    """完整流程: 查城市 -> 查天气 -> 输出。"""
    place = search_city(city_input)
    if not place:
        print("没找到城市: {}，换个写法试试（如 shanghai / 上海 / new york）".format(city_input))
        return
    weather = fetch_weather(place["latitude"], place["longitude"])
    print_weather(place, weather)


def main():
    # 避免 Windows 控制台中文输出报 UnicodeEncodeError
    try:
        sys.stdout.reconfigure(encoding="utf-8")
    except Exception:
        pass

    print("=== 城市天气查询（数据来源: Open-Meteo，输入 q 退出）===")
    while True:
        try:
            city = input("请输入城市名: ").strip()
        except (EOFError, KeyboardInterrupt):
            print("\n已退出。")
            return

        if not city or city.lower() in ("q", "quit", "exit"):
            print("已退出。")
            return

        try:
            query(city)
        except urllib.error.HTTPError as e:
            print("接口返回错误: HTTP {} {}".format(e.code, e.reason))
        except urllib.error.URLError as e:
            print("网络请求失败: {}（请检查网络或代理设置）".format(e.reason))
        except TimeoutError:
            print("请求超时，请稍后重试。")
        except json.JSONDecodeError:
            print("接口返回内容无法解析，请稍后重试。")
        except KeyError as e:
            print("接口数据缺少字段: {}".format(e))


if __name__ == "__main__":
    main()
