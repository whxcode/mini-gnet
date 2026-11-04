// Node.js 22+ 联网搜索示例：使用内置 fetch，不安装第三方依赖。
import { pathToFileURL } from "node:url";

// readArguments 从 stdin 读取完整 JSON，校验搜索词和结果条数。
async function readArguments() {
  process.stdin.setEncoding("utf8");
  let input = "";

  for await (const chunk of process.stdin) {
    input += chunk;

    if (Buffer.byteLength(input, "utf8") > 64 * 1024) {
      throw new Error("输入参数超过 64 KiB");
    }
  }

  const args = JSON.parse(input);

  if (args === null || typeof args !== "object" || Array.isArray(args)) {
    throw new Error("输入必须是 JSON 对象");
  }

  if (typeof args.query !== "string" || args.query.trim() === "") {
    throw new Error("query 必须是非空搜索词");
  }

  const limit = args.limit === undefined ? 5 : args.limit;

  if (!Number.isInteger(limit) || limit < 1 || limit > 10) {
    throw new Error("limit 必须是 1 到 10 之间的整数");
  }

  return { query: args.query.trim(), limit };
}

// decodeXML 只解码 XML 内置实体和字符编号，不加载 DTD 或外部实体。
function decodeXML(text) {
  const entities = { amp: "&", lt: "<", gt: ">", quot: '"', apos: "'" };

  return text.replace(/&(#x[0-9a-f]+|#\d+|amp|lt|gt|quot|apos);/gi, (entity, name) => {
    if (!name.startsWith("#")) {
      return entities[name.toLowerCase()];
    }

    const hexadecimal = name.slice(0, 2).toLowerCase() === "#x";
    const code = Number.parseInt(name.slice(hexadecimal ? 2 : 1), hexadecimal ? 16 : 10);

    if (code > 0x10ffff || (code >= 0xd800 && code <= 0xdfff)) {
      return entity;
    }

    return String.fromCodePoint(code);
  });
}

// fieldText 提取 RSS item 的一个文本字段，支持 CDATA 和常见的 HTML 摘要。
function fieldText(item, name) {
  const pattern = new RegExp(`<${name}\\b[^>]*>([\\s\\S]*?)<\\/${name}\\s*>`, "i");
  const match = item.match(pattern);

  if (!match) {
    return "";
  }

  let text = match[1].trim();

  if (text.startsWith("<![CDATA[") && text.endsWith("]]>")) {
    text = text.slice(9, -3);
  }

  return decodeXML(text).replace(/<[^>]*>/g, "").replace(/\s+/g, " ").trim();
}

// parseSearchResults 解析搜索源的固定 RSS 格式；验证码页或格式改变必须报错。
export function parseSearchResults(xml, limit) {
  if (
    !/<rss\b/i.test(xml) ||
    !/<channel\b/i.test(xml) ||
    !/<\/channel\s*>/i.test(xml) ||
    !/<\/rss\s*>\s*$/i.test(xml)
  ) {
    throw new Error("搜索源未返回完整 RSS，可能被限流或入口格式发生变化");
  }

  const items = [...xml.matchAll(/<item\b[^>]*>([\s\S]*?)<\/item\s*>/gi)];
  const openings = xml.match(/<item\b/gi) ?? [];

  if (items.length !== openings.length) {
    throw new Error("搜索结果条目不完整");
  }

  const results = [];
  const seen = new Set();

  for (const item of items) {
    const title = fieldText(item[1], "title");
    const link = fieldText(item[1], "link");
    const snippet = fieldText(item[1], "description");
    let url;

    try {
      url = new URL(link);
    } catch {
      continue;
    }

    if (!title || (url.protocol !== "http:" && url.protocol !== "https:")) {
      continue;
    }

    if (seen.has(url.href)) {
      continue;
    }

    seen.add(url.href);
    results.push({ title, url: url.href, snippet });

    if (results.length >= limit) {
      break;
    }
  }

  if (items.length > 0 && results.length === 0) {
    throw new Error("搜索源返回了条目，但未能提取有效标题和链接");
  }

  return results;
}

// search 等待联网响应和正文读取完成，然后返回可由 Go 校验的业务对象。
export async function search(query, limit) {
  const url = new URL("https://www.bing.com/search");
  url.searchParams.set("q", query);
  url.searchParams.set("format", "rss");

  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 15000);

  try {
    const response = await fetch(url, {
      signal: controller.signal,
      headers: { Accept: "application/rss+xml, application/xml, text/xml" },
    });

    if (!response.ok) {
      await response.body?.cancel();
      throw new Error(`搜索源返回 HTTP ${response.status}`);
    }

    const chunks = [];
    let bytes = 0;

    for await (const chunk of response.body) {
      bytes += chunk.byteLength;

      if (bytes > 2 * 1024 * 1024) {
        controller.abort();
        throw new Error("搜索响应超过 2 MiB");
      }

      chunks.push(chunk);
    }

    const xml = Buffer.concat(chunks).toString("utf8");
    const results = parseSearchResults(xml, limit);

    return {
      query,
      source: "Bing RSS",
      search_url: url.href,
      count: results.length,
      results,
    };
  } catch (error) {
    if (error.name === "AbortError") {
      throw new Error("搜索请求超时（15000 ms）");
    }

    throw new Error(error.cause?.code ?? error.message);
  } finally {
    clearTimeout(timer);
  }
}

// main 将业务结果写入 stdout，失败原因只写入 stderr。
async function main() {
  try {
    const { query, limit } = await readArguments();
    const result = await search(query, limit);

    console.log(JSON.stringify(result));
  } catch (error) {
    console.error(`联网搜索失败：${error.message}`);
    process.exitCode = 1;
  }
}

// 导入时只暴露解析和搜索函数，直接执行文件时才读取 stdin。
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  await main();
}
