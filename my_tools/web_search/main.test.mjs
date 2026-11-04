import assert from "node:assert/strict";
import test from "node:test";
import { parseSearchResults, search } from "./main.mjs";

const sample = `<?xml version="1.0"?>
<rss version="2.0"><channel>
  <item>
    <title>Go &amp; 中文</title>
    <link>https://example.com/docs?a=1&amp;b=2</link>
    <description><![CDATA[学习 <b>context</b> 与取消。]]></description>
  </item>
  <item>
    <title>重复链接</title>
    <link>https://example.com/docs?a=1&amp;b=2</link>
    <description>重复结果</description>
  </item>
  <item>
    <title>&#x4E2D;&#25991;</title>
    <link>https://example.org/</link>
    <description>第二条摘要</description>
  </item>
</channel></rss>`;

// 验证 Unicode、实体、CDATA、去重和数量限制，不访问网络。
test("解析结构化搜索结果", () => {
  const results = parseSearchResults(sample, 5);

  assert.equal(results.length, 2);
  assert.deepEqual(results[0], {
    title: "Go & 中文",
    url: "https://example.com/docs?a=1&b=2",
    snippet: "学习 context 与取消。",
  });
  assert.equal(results[1].title, "中文");
  assert.equal(parseSearchResults(sample, 1).length, 1);
});

// 空 RSS 表示没有结果；网页、验证码和不完整结果不能冒充空结果。
test("区分空结果与搜索源错误", () => {
  assert.deepEqual(parseSearchResults("<rss><channel></channel></rss>", 5), []);
  assert.throws(() => parseSearchResults("<html>captcha</html>", 5), /RSS/);
  assert.throws(() => parseSearchResults("<rss><channel></rss>", 5), /RSS/);
  assert.throws(() => parseSearchResults("<rss><channel><item></channel></rss>", 5), /不完整/);
  assert.throws(() => parseSearchResults(
    "<rss><channel><item><title>x</title><link>javascript:alert(1)</link></item></channel></rss>",
    5,
  ), /未能提取/);
});

// 延迟完成的 fetch 能验证 await 链路以及中文查询参数编码。
test("等待异步响应再返回结果", async (t) => {
  let responseReady = false;

  t.mock.method(globalThis, "fetch", async (url, options) => {
    assert.equal(url.searchParams.get("q"), "Go 中文 & context");
    assert.equal(url.searchParams.get("format"), "rss");
    assert.ok(options.signal instanceof AbortSignal);

    await new Promise((resolve) => setTimeout(resolve, 20));
    responseReady = true;

    return new Response(sample, { status: 200 });
  });

  const result = await search("Go 中文 & context", 1);

  assert.ok(responseReady);
  assert.equal(result.count, 1);
  assert.equal(result.results[0].title, "Go & 中文");
  assert.equal(result.source, "Bing RSS");
});

// HTTP 错误应进入进程错误通道，不返回伪造的搜索列表。
test("报告搜索源限流", async (t) => {
  t.mock.method(globalThis, "fetch", async () => new Response("limited", { status: 429 }));

  await assert.rejects(search("test", 5), /HTTP 429/);
});

// 网络响应必须受大小限制，不能一直累积在内存里。
test("拒绝过大的响应", async (t) => {
  t.mock.method(globalThis, "fetch", async () => new Response("x".repeat(2 * 1024 * 1024 + 1)));

  await assert.rejects(search("test", 5), /超过 2 MiB/);
});

// 模拟 fetch 的取消错误，避免测试真的等待十五秒。
test("报告请求超时", async (t) => {
  t.mock.method(globalThis, "fetch", async () => {
    throw new DOMException("aborted", "AbortError");
  });

  await assert.rejects(search("test", 5), /请求超时/);
});
