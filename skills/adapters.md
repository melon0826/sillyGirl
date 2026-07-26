---
name: sillygirl-adapter-writer
description: Use when writing, modifying, reviewing, or debugging SillyGirl platform adapters such as QQ OneBot, Telegram Bot, Web chat, Pagermaid bridges, or new chat platform integrations. Covers core.Factory lifecycle, message normalization, reply handlers, config buckets/watchers, admin checks, status visibility, and safe network behavior.
---

# SillyGirl Adapter Writer

Use this skill to build or fix SillyGirl adapters.

## Adapter Model

Adapters translate an external platform into SillyGirl's platform-neutral message model.

Core files:

- `core/adapter.go`: `Factory`, global `Bots`, adapter lookup, lifecycle.
- `core/base_sender.go`: unified `Sender` behavior.
- `core/function.go`: message filtering, rule matching, plugin dispatch.
- `adapters/qq/main.go`: OneBot reverse WebSocket adapter.
- `adapters/telegram/main.go`: Telegram Bot API long-polling adapter.
- `adapters/web/main.go`: browser long-polling web adapter.

An adapter should:

1. Read its config from a `Bucket`.
2. Register a `core.Factory`.
3. Normalize inbound platform messages to SillyGirl fields.
4. Call `adapter.Receive(params)`.
5. Implement `adapter.SetReplyHandler(...)` to send replies back to the platform.
6. Destroy the `Factory` when the connection/runtime exits.

## Factory Lifecycle

Create one `core.Factory` per connected bot identity:

```go
adapter := &core.Factory{}
adapter.Init("platform", botID, nil)
defer adapter.Destroy()

adapter.SetReplyHandler(func(msg map[string]interface{}) string {
    // send message to platform
    return messageID
})
```

Rules:

- `platform` must be stable and lowercase, for example `qq`, `telegram`, `web`.
- `botID` must uniquely identify the connected bot account.
- Always call `Destroy()` when a connection ends or runtime is cancelled.
- If the same `[platform, botID]` registers again, `Factory.Init` will destroy the old instance.
- Use `core.GetAdapterBotsID(platform)` when the UI needs available working bots.

## Normalized Message Fields

Inbound messages should use these keys:

```go
params := map[string]interface{}{
    core.USER_ID:    userID,
    core.CHAT_ID:    chatID,
    core.CONETNT:    content,
    core.MESSAGE_ID: messageID,
    "user_name":     userName,
    "chat_name":     chatName,
}
adapter.Receive(params)
```

Field expectations:

- `core.USER_ID`: required. Platform user id as string.
- `core.CHAT_ID`: group/channel/chat id as string. Empty means private chat.
- `core.CONETNT`: required. Plain message content that rules should match.
- `core.MESSAGE_ID`: message id as string when available.
- `user_name`: display name for nickname cache.
- `chat_name`: group/channel display name when available.

Use `utils.Itoa(...)`, `strconv.FormatInt(...)`, or `fmt.Sprint(...)` to normalize numeric IDs to strings.

## Nickname Cache

Record platform nicknames when the adapter sees them:

```go
core.CreateNickName(&core.Nickname{
    Value:    userName,
    ID:       userID,
    Platform: "telegram",
    BotsID:   []string{botID},
})

core.CreateNickName(&core.Nickname{
    Group:    true,
    Value:    chatName,
    ID:       chatID,
    Platform: "telegram",
    BotsID:   []string{botID},
})
```

This improves Admin display, carry rules, and message controls.

## Reply Handler

The reply handler receives SillyGirl's normalized outbound fields:

```go
adapter.SetReplyHandler(func(msg map[string]interface{}) string {
    chatID := stringValue(msg[core.CHAT_ID])
    userID := stringValue(msg[core.USER_ID])
    content := stringValue(msg[core.CONETNT])
    // send private if chatID is empty, group/channel otherwise
    return sentMessageID
})
```

Rules:

- Return the platform message id when available.
- Return `""` when sending fails or no id is available.
- Do not panic on missing or wrongly typed fields; convert safely.
- Strip or convert unsupported markup before sending.
- Log send failures with `core.Logs.Warn(...)`.

## Admin Check

If the platform can identify admin state, provide `SetIsAdmin`:

```go
adapter.SetIsAdmin(func(userID string) bool {
    return isPlatformAdmin(userID)
})
```

For web/session based adapters, derive admin from the authenticated SillyGirl session where possible. Do not trust caller-supplied `isAdmin` fields.

## Config

Use a bucket named after the adapter:

```go
var telegram = core.MakeBucket("telegram")
```

Common config keys:

- `enable`: boolean adapter switch.
- `debug`: boolean verbose logging switch.
- `token`, `access_token`, `api_key`: secrets for platform auth.
- `api_base`: optional API reverse proxy base URL.

Use `storage.Watch` to restart or refresh config:

```go
storage.Watch(bucket, "token", func(old, new, key string) *storage.Final {
    go restart()
    return nil
})
```

Do not log full tokens. Mask or omit secrets.

## Runtime Pattern

Long-running adapters should use a cancellable runtime:

```go
var runtime = struct {
    sync.Mutex
    cancel context.CancelFunc
}{}

func restart() {
    runtime.Lock()
    if runtime.cancel != nil {
        runtime.cancel()
        runtime.cancel = nil
    }
    ctx, cancel := context.WithCancel(context.Background())
    runtime.cancel = cancel
    runtime.Unlock()

    go run(ctx)
}
```

Inside loops:

- Check `ctx.Done()`.
- Back off on transient network failures.
- Do not busy-loop.
- Use `http.Client{Timeout: ...}`.
- Clean up webhooks or stale sessions only when that is the intended adapter behavior.

## HTTP and WebSocket Safety

For inbound HTTP/WebSocket adapters:

- Authenticate callback/WS connections with a configured token when possible.
- Reject bad tokens before upgrading WebSocket.
- Avoid `CheckOrigin: true` unless the endpoint also has token auth.
- Do not expose message injection endpoints by default.
- For public endpoints, require an explicit config switch and add rate limits when possible.

OneBot reverse WebSocket example:

- Endpoint: `/qq/receive`
- Token bucket key: `qq.access_token` or `qq.token`
- Bot id from `X-Self-ID`
- Register as `adapter.Init("qq", botID, nil)`

## Telegram Pattern

Telegram adapter uses Bot API long polling:

- Read `telegram.token`.
- Read `telegram.api_base`, default `https://api.telegram.org`.
- On startup, call `getMe` to get bot id.
- Register as `adapter.Init("telegram", strconv.FormatInt(me.ID, 10), nil)`.
- Poll `getUpdates` with offset.
- Send replies with `sendMessage`.

Do not use `drop_pending_updates` unless the user explicitly asks. Do not keep compatibility aliases for old token keys unless requested.

## Web Adapter Pattern

Web adapter uses `/api/web_chat` long polling:

- Register once as `adapter.Init("web", "default", nil)`.
- Use SillyGirl auth cookie/JWT to decide whether a web user is admin.
- Anonymous message injection should be disabled unless `sillyGirl.web_chat_public=true`.
- Keep per-user queues bounded.
- Expire inactive web users.

## Pagermaid Bridge

Pagermaid support is a bridge, not a native adapter in this repo:

- The bridge script is `adapters/pagermaid/sillyplus.py`.
- It should forward messages to SillyGirl through a clearly authenticated endpoint or websocket.
- SillyGirl-side support must exist before promising Pagermaid is online.
- Document the required endpoint, token, and expected message JSON shape when changing Pagermaid behavior.

## Status Visibility

Admin overview reads registered adapters from `core.Bots`.

To make a bot visible:

- Ensure `Factory.Init(platform, botID, nil)` is called after successful platform authentication.
- Ensure `Destroy()` is called on disconnect.
- Ensure bot id is non-empty and stable.
- Record nicknames with `BotsID: []string{botID}` when possible.

## Message Filtering Interaction

Adapters should deliver all legitimate user messages to `adapter.Receive`.

Do not implement plugin rule filtering inside adapters. The core router handles:

- listened groups
- no-reply groups
- blocked users
- admin commands
- plugin rule matching
- carry forwarding

Adapter-side filtering should only remove invalid, self, bot, unsupported, or empty messages.

## Testing Checklist

Before finishing adapter work:

- Adapter starts with valid config and stays stopped with missing config.
- Config changes restart or refresh the adapter.
- Inbound private message triggers a plugin.
- Inbound group/channel message includes `chat_id`.
- Self messages and bot messages are ignored.
- Reply handler sends private and group/channel replies correctly.
- Admin status is not spoofable.
- Bot appears in Admin overview and carry working-bot dropdown.
- Logs do not leak tokens.
- Network requests have timeouts.
- `go test ./...` passes.

## Common Bugs

- Forgetting `defer adapter.Destroy()`, leaving stale online bots.
- Using numeric ids directly and breaking string comparisons.
- Missing `core.CHAT_ID`, causing group messages to be treated as private.
- Trusting incoming HTTP query params for admin/user identity.
- Logging raw tokens or full Authorization headers.
- Restarting without cancelling the old goroutine.
- Blocking forever on a send response channel.
- Returning wrapped errors to users with secrets inside.
