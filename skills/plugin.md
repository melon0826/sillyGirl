---
name: sillygirl-plugin-writer
description: Use when writing, migrating, reviewing, or fixing SillyGirl NodeJS script plugins. Covers plugin metadata comments, message rules, cron/web/carry scripts, configuration schemas, sender/Bucket APIs, and inline QingLong/SmallCat/DaiDai clients.
---

# SillyGirl Plugin Writer

Use this skill to write SillyGirl script plugins for this repository.

## Ground Rules

- Write plugins as CommonJS NodeJS files.
- Import runtime APIs from `sillygirl`:

```js
const {
  sender: s,
  Bucket,
  QingLong,
  SmallCat,
  DaiDai,
  sillyGirlCreateSchema,
  SillyGirlPluginConfig,
  form,
  sleep,
  restart,
  update,
  express,
} = require('sillygirl');
```

- Do not use Goja-only APIs or BNCR globals.
- Do not use `BncrDB`, `BncrCreateSchema`, or `BncrPluginConfig`.
- Do not invent wrappers that change third-party API response shapes. Return or reply with the original API meaning unless the user asks for formatting.
- Prefer `async function main() { ... }` and end with `main().catch(...)`.
- Always handle exceptions and reply with a useful error message.
- Never hard-code secrets in plugin code. Use `SillyGirlPluginConfig`, `Bucket`, or environment variables.

## Metadata Header

Every plugin should start with a block comment:

```js
/**
 * @title 插件标题
 * @author 作者
 * @version v1.0.0
 * @desc 插件说明
 * @rule raw ^命令$
 * @admin false
 * @priority 100
 */
```

Supported metadata:

| Tag | Required | Meaning |
| --- | --- | --- |
| `@title 标题` | Recommended | Display name in Admin and plugin market. |
| `@author 作者` | Optional | Plugin author. |
| `@version v1.0.0` | Optional | Plugin version. |
| `@desc 说明` | Optional | Plugin description. Use `@desc`, not `@description`. |
| `@rule 规则` | Required for message plugins | Message trigger. Can appear multiple times. |
| `@admin true/false` | Optional | Whether only admins can trigger it. |
| `@priority 数字` | Optional | Match priority; lower/higher behavior follows project parser. |
| `@cron 表达式` | Required for cron plugins | Cron expression only, for example `@cron 0 9 * * *`. Do not append platform. |
| `@web true/false` | Required for web daemon plugins | Whether the plugin stays running. Express must listen on its own port in code. |
| `@carry true` | Required for carry handlers | Makes the plugin selectable as a carry processing script. |
| `@module true` | Optional | Utility/module file, not a normal message handler. |
| `@on_start true` | Optional | Run once on startup. |

Do not use these removed/legacy tags:

- `@name`
- `@disable`
- `@message`
- `@service`
- `@create_at`
- `@form`
- `@encrypt`
- `@paterner`
- `@http`
- `@findall`
- `@match`
- `@regex`
- `@pattern`
- `@groupId`, `@groupId-`
- `@userId`, `@userId-`
- `@platform`, `@platform-`

## Rule Patterns

Use simple rules unless exact regex is needed:

```js
/**
 * @title 示例
 * @rule raw ^ping$
 * @rule 天气 [城市]
 */
```

- Use `raw ^...$` for exact regex.
- Use `[参数名]` for captured parameters.
- Read captured text from `s.param(index)` when needed.
- If the script has no `@rule`, it will not be triggered by normal messages.

## Sender API

Use `sender` as `s`:

```js
const content = await s.getContent();
const userId = await s.getUserId();
const chatId = await s.getChatId();
const isAdmin = await s.isAdmin();
await s.reply('回复内容');
```

Common methods:

- `s.getContent()`
- `s.getUserId()`
- `s.getChatId()`
- `s.getPlatform()`
- `s.getBotId()`
- `s.isAdmin()`
- `s.param(index)`
- `s.reply(text)`

For admin-only commands, still check `await s.isAdmin()` in code when the action is sensitive, even if `@admin true` is present.

## Storage

Use `Bucket` for persistent plugin data:

```js
const db = new Bucket('my-plugin');
const oldValue = await db.get('key', '');
await db.set('key', 'value');
await db.delete('key');
```

Use one bucket per plugin or feature. Avoid writing to shared buckets like `sillyGirl` unless the user explicitly asks.

## Plugin Configuration

Use `sillyGirlCreateSchema` plus `new SillyGirlPluginConfig(schema)` or `form(schema)` at top level.

```js
const schema = sillyGirlCreateSchema.object({
  apiBase: sillyGirlCreateSchema.string().title('接口地址').default(''),
  token: sillyGirlCreateSchema.string().title('Token').format('password').default(''),
});

const Config = new SillyGirlPluginConfig(schema);
```

Read config values:

```js
const apiBase = await Config.get('apiBase', '');
const token = await Config.get('token', '');
```

Config registration must run at plugin load time, not inside a branch that may never execute.

## Inline Clients

Constructors use object parameters only:

```js
const ql = new QingLong({ id: 1 });
const sc = new SmallCat({ id: 1 });
const dd = new DaiDai({ id: 1 });
```

Do not write:

```js
new QingLong(1);
new SmallCat(1);
new DaiDai(1);
```

### QingLong

Common methods:

- `ql.getEnvs(search?)`
- `ql.getEnvById(id)`
- `ql.createEnv({ name, value, remarks })`
- `ql.updateEnv({ id, name, value, remarks })`
- `ql.deleteEnvs(ids)`
- `ql.enableEnvs(ids)`
- `ql.disableEnvs(ids)`
- `ql.systemNotify(title, content)`

### SmallCat

Common methods:

- `sc.createQr(type)`
- `sc.checkQr(uuid)`
- `sc.addUser({ code, type, displayName })`
- `sc.userList()`
- `sc.getCode({ openid, appid })`
- `sc.getUserInfo({ openid, appid })`
- `sc.getPhoneNumber({ openid, appid })`
- `sc.qrCodeAuth(payload)`
- `sc.oAuth(payload)`

Use camelCase exactly. Do not write `chechQr`.

### DaiDai

Common methods should follow the project inline client implementation. If unsure, inspect `core/node_runtime_preload.go` before using a method.

## Web Plugins

For HTTP plugins:

```js
/**
 * @title Web 示例
 * @web true
 * @version v1.0.0
 */

const { express } = require('sillygirl');
const app = express();

app.get('/health', (_req, res) => {
  res.json({ status: true, message: 'ok', data: null });
});

app.listen(3001, () => {
  console.log('web plugin listening on 3001');
});
```

`@web` only accepts `true` or `false`. Do not put a port in the metadata.

## Carry Plugins

Carry handlers must include `@carry true` so Admin can select them:

```js
/**
 * @title 搬运处理
 * @carry true
 * @version v1.0.0
 */

const { sender: s } = require('sillygirl');

async function main() {
  const content = await s.getContent();
  await s.reply(content);
}

main().catch(err => s.reply(`搬运处理失败：${err.message || err}`));
```

## Cron Plugins

Cron plugins should include `@cron`:

```js
/**
 * @title 每日提醒
 * @cron 0 9 * * *
 * @version v1.0.0
 */

const { sender: s } = require('sillygirl');

async function main() {
  await s.reply('每日提醒');
}

main().catch(err => console.error(err));
```

## Response Style

- For chat commands, reply in concise Chinese.
- For API helper plugins, preserve original JSON response when the user needs raw results.
- For status summaries, include enough fields to debug: status, message, id/openid/uuid when relevant.
- Mask secrets before replying.

## Final Checklist

Before finishing a plugin:

- Header uses `@title`, not `@name`.
- Description uses `@desc`, not `@description`.
- No BNCR names remain.
- Constructors are `new QingLong({ id })`, `new SmallCat({ id })`, `new DaiDai({ id })`.
- `SmallCat.checkQr` is spelled correctly.
- Sensitive commands check admin permission.
- External requests have timeouts or are wrapped in try/catch.
- Config schema is registered at top level if configuration is needed.
- Code is valid CommonJS and can run under NodeJS.
