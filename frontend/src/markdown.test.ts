// Run with `npm --prefix frontend test` — plain node, because the renderer is a
// pure function and a browser is not needed to know that a model's `**译**` has to
// come out as <strong>.
import { renderMarkdown } from './markdown.ts'

const cases: [string, string, string][] = [
  ['段落与列表', '**译**\n- 一\n- 二\n\n结尾',
    '<p><strong>译</strong></p><ul><li>一</li><li>二</li></ul><p>结尾</p>'],
  ['标题与代码', '### 标题\n用 `map` 映射\n\n```js\nconst a = 1\n```\n',
    '<h3>标题</h3><p>用 <code>map</code> 映射</p><pre><code>const a = 1</code></pre>'],
  ['有序列表', '1. 甲\n2. 乙', '<ol><li>甲</li><li>乙</li></ol>'],
  ['引用与分隔线', '> 引用\n\n---\n正文', '<blockquote>引用</blockquote><hr><p>正文</p>'],
  ['换行留在段内', '第一行\n第二行', '<p>第一行<br>第二行</p>'],
  ['引用没有闭合也照常收尾', '```js\nlet a\n', '<pre><code>let a</code></pre>'],
  ['普通译文原样保留', '这是一句普通的翻译结果。', '<p>这是一句普通的翻译结果。</p>'],

  // What the escaping is for: an answer that carries markup must not gain a script,
  // a style sheet or a link that leaves the web.
  ['回答里的标签只是文字', '安全 <script>alert(1)</script>',
    '<p>安全 &lt;script&gt;alert(1)&lt;/script&gt;</p>'],
  ['回答里的样式表也是文字', '<style>body{display:none}</style>',
    '<p>&lt;style&gt;body{display:none}&lt;/style&gt;</p>'],
  ['非 web 的链接不变成锚点', '[点我](javascript:alert(1))', '<p>[点我](javascript:alert(1))</p>'],
  ['http 链接保留', '看 [官网](https://example.com/a?b=1)',
    '<p>看 <a href="https://example.com/a?b=1">官网</a></p>'],
  ['星号乘号不误作斜体', '3 * 4 * 5 等于 12', '<p>3 * 4 * 5 等于 12</p>'],
  ['紧贴文字的星号才是斜体', '这是 *重点* 内容', '<p>这是 <em>重点</em> 内容</p>'],
]

let failures = 0
for (const [name, input, want] of cases) {
  const got = renderMarkdown(input)
  if (got !== want) {
    failures++
    console.error(`FAIL ${name}\n  输入 ${JSON.stringify(input)}\n  得到 ${got}\n  期望 ${want}`)
  }
}
if (failures) throw new Error(`${failures}/${cases.length} 个 Markdown 用例未通过`)
console.log(`ok — ${cases.length} 个 Markdown 用例通过`)
