import asyncio
import base64
import inspect
import json
import os
import pickle
import re
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request

import grpc

import srpc_pb2
import srpc_pb2_grpc


plugin_id = os.environ.get("PLUGIN_ID", "")
runtime_id = os.environ.get("RUNTIME_ID", "")
grpc_addr = os.environ.get("SILLYGIRL_GRPC_ADDR", "127.0.0.1:50051")
grpc_token = os.environ.get("SILLYGIRL_GRPC_TOKEN", "")
metadata = (("runtime_id", runtime_id), ("sillygirl-runtime-token", grpc_token))

_channel = None
_stub = None
_async_channel = None
_async_stub = None


def get_stub():
    global _channel, _stub
    if _stub is None:
        _channel = grpc.insecure_channel(grpc_addr)
        _stub = srpc_pb2_grpc.SillyGirlServiceStub(_channel)
    return _stub


def get_async_stub():
    global _async_channel, _async_stub
    if _async_stub is None:
        _async_channel = grpc.aio.insecure_channel(grpc_addr)
        _async_stub = srpc_pb2_grpc.SillyGirlServiceStub(_async_channel)
    return _async_stub


def transform(value):
    if not value:
        return None
    if value.startswith("f:"):
        return float(value[2:])
    if value.startswith("d:") or value.startswith("i:"):
        return int(value[2:])
    if value.startswith("b:"):
        return value[2:] == "true"
    if value.startswith("o:"):
        return json.loads(value[2:])
    if value.startswith("p:"):
        return pickle.loads(base64.b64decode(value[2:]))
    return value


def reverse_transform(value):
    try:
        if isinstance(value, bool):
            return "b:true" if value else "b:false"
        if isinstance(value, int):
            return f"d:{value}"
        if isinstance(value, float):
            return f"f:{value}"
        if isinstance(value, str):
            return value
        if value is None:
            return ""
        return "o:" + json.dumps(value, ensure_ascii=False, separators=(",", ":"))
    except Exception:
        return "p:%s" % base64.b64encode(pickle.dumps(value)).decode("utf-8")


reverseTransform = reverse_transform


class Bucket:
    def __init__(self, name):
        self.__name = str(name)

    def __getitem__(self, key):
        return self.__get(key)

    def __getattr__(self, name):
        return self.__get(name)

    def __setitem__(self, key, value):
        self.__set(key, value)

    def __setattr__(self, name, value):
        if name == "_Bucket__name":
            object.__setattr__(self, name, value)
        else:
            self.__set(name, value)

    async def get(self, key, defaultValue=None):
        response = await get_async_stub().BucketGet(
            srpc_pb2.BucketKeyRequest(name=self.__name, key=str(key)),
            metadata=metadata,
        )
        value = transform(response.value)
        return defaultValue if value is None else value

    def __get(self, key, defaultValue=None):
        response = get_stub().BucketGet(
            srpc_pb2.BucketKeyRequest(name=self.__name, key=str(key)),
            metadata=metadata,
        )
        value = transform(response.value)
        return defaultValue if value is None else value

    async def set(self, key, value):
        response = await get_async_stub().BucketSet(
            srpc_pb2.BucketSetRequest(name=self.__name, key=str(key), value=reverse_transform(value)),
            metadata=metadata,
        )
        return {"message": response.message, "changed": response.changed}

    def __set(self, key, value):
        response = get_stub().BucketSet(
            srpc_pb2.BucketSetRequest(name=self.__name, key=str(key), value=reverse_transform(value)),
            metadata=metadata,
        )
        return {"message": response.message, "changed": response.changed}

    async def getAll(self):
        response = await get_async_stub().BucketGetAll(
            srpc_pb2.BucketRequest(name=self.__name),
            metadata=metadata,
        )
        raw = json.loads(response.value or "{}")
        return {key: transform(value) for key, value in raw.items()}

    async def delete(self, key):
        return await self.set(key, None)

    async def deleteAll(self):
        await get_async_stub().BucketDelete(
            srpc_pb2.BucketRequest(name=self.__name),
            metadata=metadata,
        )

    async def keys(self):
        response = await get_async_stub().BucketKeys(
            srpc_pb2.BucketRequest(name=self.__name),
            metadata=metadata,
        )
        return list(response.keys)

    async def len(self):
        response = await get_async_stub().BucketLen(
            srpc_pb2.BucketRequest(name=self.__name),
            metadata=metadata,
        )
        return response.length

    async def buckets(self):
        response = await get_async_stub().BucketBuckets(
            srpc_pb2.Empty(),
            metadata=metadata,
        )
        return list(response.buckets)

    async def getName(self):
        return self.__name

    def watch(self, key, handle):
        async def watch_loop():
            queue = asyncio.Queue()

            async def request_iterator():
                yield srpc_pb2.BucketWatchRequest(
                    name=self.__name,
                    key=str(key),
                    plugin_id=plugin_id,
                )
                while True:
                    item = await queue.get()
                    if item is None:
                        return
                    yield item

            async for response in get_async_stub().BucketWatch(request_iterator(), metadata=metadata):
                try:
                    result = handle(transform(response.old), transform(response.now), response.key)
                    if inspect.isawaitable(result):
                        result = await result
                except Exception as exc:
                    result = {"error": str(exc)}

                payload = {"echo": response.echo}
                if not result:
                    payload["error"] = "VOID"
                else:
                    if "now" in result:
                        payload["now"] = reverse_transform(result["now"])
                    if "message" in result:
                        payload["message"] = str(result["message"])
                    if "error" in result:
                        payload["error"] = str(result["error"])
                await queue.put(srpc_pb2.BucketWatchRequest(**payload))

        try:
            asyncio.get_running_loop().create_task(watch_loop())
        except RuntimeError:
            asyncio.get_event_loop().create_task(watch_loop())


def normalize_schema(value):
    if isinstance(value, SchemaNode):
        return value.toJSON()
    if hasattr(value, "toJSON") and callable(value.toJSON):
        return value.toJSON()
    if isinstance(value, list):
        return [normalize_schema(item) for item in value]
    if isinstance(value, dict):
        return {key: normalize_schema(item) for key, item in value.items() if not str(key).startswith("_")}
    return value


def pluginConfigDefaults(schema):
    schema = normalize_schema(schema) or {}
    if "default" in schema:
        return schema["default"]
    if schema.get("type") == "object" or schema.get("properties"):
        result = {}
        for key, value in (schema.get("properties") or {}).items():
            default_value = pluginConfigDefaults(value)
            if default_value is not None:
                result[key] = default_value
        return result
    if schema.get("type") == "array":
        return []
    return None


class SchemaNode:
    def __init__(self, schema_type, extra=None):
        self.schema = {"type": schema_type}
        if extra:
            self.schema.update(extra)

    def setTitle(self, value):
        self.schema["title"] = value
        return self

    def setDescription(self, value):
        self.schema["description"] = value
        return self

    def setDefault(self, value):
        self.schema["default"] = value
        return self

    def setEnum(self, value):
        self.schema["enum"] = value
        return self

    def setEnumNames(self, value):
        self.schema["enumNames"] = value
        return self

    def setRequired(self, value):
        self.schema["required"] = value
        return self

    def setFormat(self, value):
        self.schema["format"] = value
        return self

    def setMin(self, value):
        self.schema["minimum"] = value
        return self

    def setMax(self, value):
        self.schema["maximum"] = value
        return self

    def setMinLength(self, value):
        self.schema["minLength"] = value
        return self

    def setMaxLength(self, value):
        self.schema["maxLength"] = value
        return self

    def setPattern(self, value):
        self.schema["pattern"] = value
        return self

    def setWidget(self, value):
        self.schema["ui:widget"] = value
        return self

    def toJSON(self):
        return self.schema


class _SillyGirlCreateSchema:
    def string(self):
        return SchemaNode("string")

    def number(self):
        return SchemaNode("number")

    def integer(self):
        return SchemaNode("integer")

    def boolean(self):
        return SchemaNode("boolean")

    def array(self, item=None):
        return SchemaNode("array", {"items": normalize_schema(item) or {}})

    def object(self, props=None):
        return SchemaNode(
            "object",
            {"properties": {key: normalize_schema(value) for key, value in (props or {}).items()}},
        )


sillyGirlCreateSchema = _SillyGirlCreateSchema()


class SillyGirlPluginConfig:
    def __init__(self, schema):
        self.uuid = plugin_id
        self.jsonSchema = normalize_schema(schema) or {}
        if not self.jsonSchema.get("type"):
            self.jsonSchema["type"] = "object"
        self.userConfig = {}
        if os.environ.get("PLUGIN_CONFIG_JSON"):
            try:
                value = json.loads(os.environ["PLUGIN_CONFIG_JSON"])
                if isinstance(value, dict):
                    self.userConfig = value
            except Exception:
                pass
        if os.environ.get("SILLYGIRL_CONFIG_REGISTER_ONLY") == "true":
            target = os.environ.get("SILLYGIRL_CONFIG_SCHEMA_FILE", "")
            if target:
                with open(target, "w", encoding="utf-8") as fp:
                    json.dump(self.jsonSchema, fp, ensure_ascii=False, separators=(",", ":"))
            else:
                print("__SILLYGIRL_CONFIG_SCHEMA__" + json.dumps(self.jsonSchema, ensure_ascii=False))
            os._exit(0)

    async def init(self):
        if not self.uuid:
            return self.userConfig
        await Bucket("plugin_config_schemas").set(self.uuid, self.jsonSchema)
        self.userConfig = await Bucket("plugin_config_values").get(self.uuid, {})
        return self.userConfig

    async def get(self):
        if self.uuid:
            self.userConfig = await Bucket("plugin_config_values").get(self.uuid, {})
        return self.userConfig

    async def Get(self):
        return await self.get()

    async def set(self, values=None):
        if isinstance(values, dict):
            self.userConfig = values
        await Bucket("plugin_config_values").set(self.uuid, self.userConfig or {})
        return {"error": ""}

    async def Set(self, values=None):
        return await self.set(values)


def form(schema):
    return SillyGirlPluginConfig(schema)


async def _read_runtime_panels(key):
    raw = await Bucket("sillyGirl").get(key, [])
    if isinstance(raw, list):
        return raw
    if isinstance(raw, str) and raw.strip():
        text = raw[2:] if raw.startswith("o:") else raw
        try:
            value = json.loads(text)
            return value if isinstance(value, list) else []
        except Exception:
            return []
    return []


def _runtime_panel_index(ref):
    if isinstance(ref, dict):
        ref = ref.get("id") or ref.get("ID")
    try:
        return int(ref)
    except Exception:
        return 0


def _normalize_path(value, prefix):
    value = str(value or "").strip()
    if not value:
        value = prefix
    if not value.startswith("/"):
        value = "/" + value
    if prefix and value != prefix and not value.startswith(prefix + "/"):
        value = prefix + value
    return value


def _query_string(query=None):
    query = query or {}
    values = urllib.parse.urlencode({key: value for key, value in query.items() if value is not None})
    return "?" + values if values else ""


def _normalize_ids(ids):
    if isinstance(ids, list):
        return ids
    if isinstance(ids, str):
        values = []
        for item in re.split(r"[,\s]+", ids):
            item = item.strip()
            if not item:
                continue
            values.append(int(item) if re.fullmatch(r"-?\d+", item) else item)
        return values
    return [ids]


def _http_json_sync(method, url, headers=None, body=None):
    headers = dict(headers or {})
    data = None
    if body is not None:
        data = json.dumps(body, ensure_ascii=False).encode("utf-8")
        headers.setdefault("Content-Type", "application/json")
    request = urllib.request.Request(url, data=data, headers=headers, method=str(method or "GET").upper())
    try:
        with urllib.request.urlopen(request, timeout=30) as response:
            raw = response.read().decode("utf-8", "replace")
            status = getattr(response, "status", 200)
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode("utf-8", "replace")
        status = exc.code
    payload = json.loads(raw) if raw.strip() else {}
    if status < 200 or status >= 300:
        raise RuntimeError(payload.get("message") or payload.get("error") or f"HTTP {status}")
    return payload


async def _http_json(method, url, headers=None, body=None):
    return await asyncio.to_thread(_http_json_sync, method, url, headers, body)


class QingLong:
    def __init__(self, options):
        self.id = _runtime_panel_index(options)
        self.uuid = ""
        self.name = ""
        self.address = ""
        self.panel = None
        self.token = ""
        self.expiration = 0

    async def _ready(self):
        if self.panel is not None:
            return
        panels = await _read_runtime_panels("qinglong_panels")
        if self.id < 1 or self.id > len(panels):
            raise RuntimeError(f"青龙编号 {self.id or ''} 不存在")
        self.panel = panels[self.id - 1]
        self.uuid = self.panel.get("id", "")
        self.name = self.panel.get("name", "")
        self.address = str(self.panel.get("address", "")).rstrip("/")

    async def _ensure_token(self):
        await self._ready()
        now = int(time.time())
        if self.token and self.expiration > now + 60:
            return
        result = await _http_json(
            "GET",
            self.address
            + "/open/auth/token"
            + _query_string({"client_id": self.panel.get("client_id"), "client_secret": self.panel.get("client_secret")}),
        )
        data = result.get("data") or {}
        if result.get("code") != 200 or not data.get("token"):
            raise RuntimeError(result.get("message") or "青龙认证失败")
        self.token = data["token"]
        self.expiration = int(data.get("expiration") or 0)

    async def request(self, method, path, body=None, query=None):
        await self._ensure_token()
        result = await _http_json(
            method,
            self.address + _normalize_path(path, "/open") + _query_string(query),
            {"Authorization": f"Bearer {self.token}"},
            body,
        )
        if "code" in result and result.get("code") != 200:
            raise RuntimeError(result.get("message") or "青龙接口请求失败")
        return result

    async def getEnvs(self, options=None):
        query = {"searchValue": options} if isinstance(options, str) else (options or {})
        result = await self.request("GET", "/envs", None, query)
        return result.get("data", result)

    async def getEnvById(self, env_id):
        result = await self.request("GET", f"/envs/{env_id}")
        return result.get("data", result)

    async def createEnv(self, env):
        result = await self.request("POST", "/envs", env if isinstance(env, list) else [env])
        return result.get("data", result)

    async def updateEnv(self, env):
        result = await self.request("PUT", "/envs", env)
        return result.get("data", result)

    async def deleteEnvs(self, ids):
        result = await self.request("DELETE", "/envs", _normalize_ids(ids))
        return result.get("data", result)

    async def disableEnvs(self, ids):
        result = await self.request("PUT", "/envs/disable", _normalize_ids(ids))
        return result.get("data", result)

    async def enableEnvs(self, ids):
        result = await self.request("PUT", "/envs/enable", _normalize_ids(ids))
        return result.get("data", result)

    async def systemNotify(self, title, content):
        result = await self.request("PUT", "/system/notify", {"title": title, "content": content})
        return result.get("data", result)


class SmallCat:
    def __init__(self, options):
        self.id = _runtime_panel_index(options)
        self.uuid = ""
        self.name = ""
        self.address = ""
        self.panel = None

    async def _ready(self):
        if self.panel is not None:
            return
        panels = await _read_runtime_panels("smallcat_panels")
        if self.id < 1 or self.id > len(panels):
            raise RuntimeError(f"smallcat 编号 {self.id or ''} 不存在")
        self.panel = panels[self.id - 1]
        self.uuid = self.panel.get("id", "")
        self.name = self.panel.get("name", "")
        self.address = str(self.panel.get("address", "")).rstrip("/")

    async def request(self, method, path, body=None, query=None):
        await self._ready()
        return await _http_json(
            method,
            self.address + _normalize_path(path, "") + _query_string(query),
            {"auth": str(self.panel.get("api_auth") or "")},
            body,
        )

    async def createQr(self, qr_type):
        return await self.request("POST", "/api/qr/start", qr_type if isinstance(qr_type, dict) else {"type": qr_type})

    async def checkQr(self, uuid):
        return await self.request("GET", "/api/qr/status", None, {"uuid": uuid})

    async def addUser(self, options):
        return await self.request("POST", "/api/accounts/add", options or {})

    async def userList(self):
        return await self.request("GET", "/api/accounts")

    async def getCode(self, options):
        return await self.request("POST", "/wx/code", dict(options or {}))

    async def getUserInfo(self, options):
        return await self.request("POST", "/wx/getuserinfo", dict(options or {}))

    async def getPhoneNumber(self, options):
        return await self.request("POST", "/wx/getphonenumber", dict(options or {}))

    async def qrCodeAuth(self, options):
        return await self.request("POST", "/wx/qrcodeauth", dict(options or {}))

    async def oAuth(self, options):
        return await self.request("POST", "/wx/oauth", dict(options or {}))


class DaiDai:
    def __init__(self, options):
        self.id = _runtime_panel_index(options)
        self.uuid = ""
        self.name = ""
        self.address = ""
        self.panel = None
        self.token = ""
        self.expiration = 0

    async def _ready(self):
        if self.panel is not None:
            return
        panels = await _read_runtime_panels("daidai_panels")
        if self.id < 1 or self.id > len(panels):
            raise RuntimeError(f"呆呆面板编号 {self.id or ''} 不存在")
        self.panel = panels[self.id - 1]
        self.uuid = self.panel.get("id", "")
        self.name = self.panel.get("name", "")
        self.address = str(self.panel.get("address", "")).rstrip("/")

    async def _ensure_token(self):
        await self._ready()
        now = int(time.time())
        if self.token and self.expiration > now + 60:
            return
        result = await _http_json(
            "POST",
            self.address + "/api/open-api/token",
            None,
            {"app_key": self.panel.get("app_key"), "app_secret": self.panel.get("app_secret")},
        )
        data = result.get("data") or {}
        if not data.get("access_token"):
            raise RuntimeError(result.get("message") or result.get("error") or "呆呆面板认证失败")
        self.token = data["access_token"]
        self.expiration = now + int(data.get("expires_in") or 86400)

    async def request(self, method, path, body=None, query=None):
        await self._ensure_token()
        result = await _http_json(
            method,
            self.address + _normalize_path(path, "/api") + _query_string(query),
            {"Authorization": f"Bearer {self.token}"},
            body,
        )
        if result.get("success") is False:
            raise RuntimeError(result.get("message") or result.get("error") or "呆呆面板接口请求失败")
        return result

    async def getEnvs(self, options=None):
        query = {"keyword": options} if isinstance(options, str) else (options or {})
        result = await self.request("GET", "/envs", None, query)
        return result.get("data", result)

    async def getEnvById(self, env_id):
        result = await self.request("GET", f"/envs/{env_id}")
        return result.get("data", result)

    async def createEnv(self, env):
        result = await self.request("POST", "/envs", env)
        return result.get("data", result)

    async def updateEnv(self, env):
        body = dict(env or {})
        env_id = body.pop("id", body.pop("ID", ""))
        result = await self.request("PUT", f"/envs/{env_id}" if env_id else "/envs", body)
        return result.get("data", result)

    async def deleteEnv(self, env_id):
        return await self.request("DELETE", f"/envs/{env_id}")

    async def deleteEnvs(self, ids):
        return await self.request("DELETE", "/envs/batch", {"ids": _normalize_ids(ids)})

    async def enableEnv(self, env_id):
        result = await self.request("PUT", f"/envs/{env_id}/enable")
        return result.get("data", result)

    async def disableEnv(self, env_id):
        result = await self.request("PUT", f"/envs/{env_id}/disable")
        return result.get("data", result)

    async def enableEnvs(self, ids):
        return await self.request("PUT", "/envs/batch/enable", {"ids": _normalize_ids(ids)})

    async def disableEnvs(self, ids):
        return await self.request("PUT", "/envs/batch/disable", {"ids": _normalize_ids(ids)})

    async def getTasks(self, options=None):
        query = {"keyword": options} if isinstance(options, str) else (options or {})
        result = await self.request("GET", "/tasks", None, query)
        return result.get("data", result)

    async def getTaskById(self, task_id):
        result = await self.request("GET", f"/tasks/{task_id}")
        return result.get("data", result)

    async def createTask(self, task):
        result = await self.request("POST", "/tasks", task)
        return result.get("data", result)

    async def updateTask(self, task):
        body = dict(task or {})
        task_id = body.pop("id", body.pop("ID", ""))
        result = await self.request("PUT", f"/tasks/{task_id}" if task_id else "/tasks", body)
        return result.get("data", result)

    async def deleteTask(self, task_id):
        return await self.request("DELETE", f"/tasks/{task_id}")

    async def runTask(self, task_id):
        return await self.request("PUT", f"/tasks/{task_id}/run")

    async def stopTask(self, task_id):
        return await self.request("PUT", f"/tasks/{task_id}/stop")

    async def enableTask(self, task_id):
        return await self.request("PUT", f"/tasks/{task_id}/enable")

    async def disableTask(self, task_id):
        return await self.request("PUT", f"/tasks/{task_id}/disable")

    async def systemNotify(self, title, content):
        return await self.request("POST", "/notifications/send", {"title": title, "content": content})


class Sender:
    def __init__(self, uuid):
        self.__uuid = uuid
        self.destroyed = False

    async def destroy(self):
        if self.destroyed:
            return
        self.destroyed = True
        await get_async_stub().SenderDestroy(
            srpc_pb2.ReplyRequest(uuid=self.__uuid),
            metadata=metadata,
        )

    async def getUserId(self):
        response = await get_async_stub().SenderGetUserId(srpc_pb2.SenderRequest(uuid=self.__uuid), metadata=metadata)
        return response.value

    async def getUserName(self):
        response = await get_async_stub().SenderGetUserName(srpc_pb2.SenderRequest(uuid=self.__uuid), metadata=metadata)
        return response.value

    async def getChatId(self):
        response = await get_async_stub().SenderGetChatId(srpc_pb2.SenderRequest(uuid=self.__uuid), metadata=metadata)
        return response.value

    async def getChatName(self):
        response = await get_async_stub().SenderGetChatName(srpc_pb2.SenderRequest(uuid=self.__uuid), metadata=metadata)
        return response.value

    async def getMessageId(self):
        response = await get_async_stub().SenderGetMessageId(srpc_pb2.SenderRequest(uuid=self.__uuid), metadata=metadata)
        return response.value

    async def getPlatform(self):
        response = await get_async_stub().SenderGetPlatform(srpc_pb2.SenderRequest(uuid=self.__uuid), metadata=metadata)
        return response.value

    async def getBotId(self):
        response = await get_async_stub().SenderGetBotId(srpc_pb2.SenderRequest(uuid=self.__uuid), metadata=metadata)
        return response.value

    async def getContent(self):
        response = await get_async_stub().SenderGetContent(srpc_pb2.SenderRequest(uuid=self.__uuid), metadata=metadata)
        return response.value

    async def isAdmin(self):
        response = await get_async_stub().SenderIsAdmin(srpc_pb2.SenderRequest(uuid=self.__uuid), metadata=metadata)
        return response.value

    async def param(self, key):
        response = await get_async_stub().SenderParam(
            srpc_pb2.ReplyRequest(uuid=self.__uuid, content=str(key)),
            metadata=metadata,
        )
        return response.value

    async def setContent(self, content):
        await get_async_stub().SenderSetContent(
            srpc_pb2.SenderContentRequest(uuid=self.__uuid, content=str(content)),
            metadata=metadata,
        )

    async def continue_(self):
        await get_async_stub().SenderContinue(srpc_pb2.SenderRequest(uuid=self.__uuid), metadata=metadata)

    async def getEvent(self):
        response = await get_async_stub().SenderEvent(srpc_pb2.SenderRequest(uuid=self.__uuid), metadata=metadata)
        return json.loads(response.value or "{}")

    async def getAdapter(self):
        return Adapter(await self.getPlatform(), await self.getBotId())

    async def listen(
        self,
        options=None,
        timeout=0,
        rules=None,
        handle=None,
        listen_private=False,
        listen_group=False,
        allow_platforms=None,
        prohibit_platforms=None,
        allow_groups=None,
        prohibit_groups=None,
        allow_users=None,
        prohibit_users=None,
    ):
        if isinstance(options, dict):
            timeout = options.get("timeout", timeout)
            rules = options.get("rules", rules)
            handle = options.get("handle", handle)
            listen_private = options.get("listen_private", listen_private)
            listen_group = options.get("listen_group", listen_group)
            allow_platforms = options.get("allow_platforms", allow_platforms)
            prohibit_platforms = options.get("prohibit_platforms", prohibit_platforms)
            allow_groups = options.get("allow_groups", allow_groups)
            prohibit_groups = options.get("prohibit_groups", prohibit_groups)
            allow_users = options.get("allow_users", allow_users)
            prohibit_users = options.get("prohibit_users", prohibit_users)

        queue = asyncio.Queue()

        async def request_iterator():
            yield srpc_pb2.SenderListenRequest(
                uuid=self.__uuid,
                timeout=int(timeout or 0),
                rules=list(rules or []),
                listen_private=bool(listen_private),
                listen_group=bool(listen_group),
                allow_platforms=list(allow_platforms or []),
                prohibit_platforms=list(prohibit_platforms or []),
                allow_groups=list(allow_groups or []),
                prohibit_groups=list(prohibit_groups or []),
                allow_users=list(allow_users or []),
                prohibit_users=list(prohibit_users or []),
                persistent=self.__uuid == "",
                plugin_id=plugin_id,
            )
            while True:
                item = await queue.get()
                if item is None:
                    return
                yield item

        result_sender = None
        async for response in get_async_stub().SenderListen(request_iterator(), metadata=metadata):
            if response.echo == "END":
                break
            result_sender = Sender(response.uuid) if response.uuid else None
            value = ""
            if handle is not None and result_sender is not None:
                value = handle(result_sender)
                if inspect.isawaitable(value):
                    value = await value
                value = "" if value is None else str(value)
            await queue.put(srpc_pb2.SenderListenRequest(uuid=response.echo, value=value))
        await queue.put(None)
        return result_sender

    def holdOn(self, value):
        return "go_again_" + str(value)

    async def reply(self, content):
        response = await get_async_stub().SenderReply(
            srpc_pb2.ReplyRequest(uuid=self.__uuid, content=str(content)),
            metadata=metadata,
        )
        return response.value

    async def doAction(self, properties):
        response = await get_async_stub().SenderAction(
            srpc_pb2.ReplyRequest(uuid=self.__uuid, content=json.dumps(properties or {}, ensure_ascii=False)),
            metadata=metadata,
        )
        return json.loads(response.value or "null")


setattr(Sender, "continue", Sender.continue_)
sender = Sender(os.environ.get("SENDER_ID", ""))
s = sender


class Adapter:
    def __init__(self, platform=None, bot_id="", replyHandler=None, actionHandler=None, **kwargs):
        if isinstance(platform, dict):
            options = platform
            platform = options.get("platform", "")
            bot_id = options.get("bot_id", options.get("botId", ""))
            replyHandler = options.get("replyHandler", replyHandler)
            actionHandler = options.get("actionHandler", actionHandler)
        self.platform = str(platform or "")
        self.bot_id = str(bot_id or "")
        self.queue = None
        self.task = None
        if replyHandler is not None or actionHandler is not None:
            try:
                self.task = asyncio.get_running_loop().create_task(self.__run(replyHandler, actionHandler))
            except RuntimeError:
                self.task = asyncio.get_event_loop().create_task(self.__run(replyHandler, actionHandler))

    async def __run(self, replyHandler, actionHandler):
        self.queue = asyncio.Queue()

        async def request_iterator():
            yield srpc_pb2.AdapterRegistRequest(bot_id=self.bot_id, platform=self.platform)
            while True:
                item = await self.queue.get()
                if item is None:
                    return
                yield item

        async for response in get_async_stub().AdapterRegist(request_iterator(), metadata=metadata):
            message = json.loads(response.value or "{}")
            echo = message.pop("echo", "")
            message_type = message.pop("__type__", "")
            handler = replyHandler if message_type == "reply" else actionHandler
            value = ""
            if handler is not None:
                value = handler(message)
                if inspect.isawaitable(value):
                    value = await value
                value = "" if value is None else str(value)
            await self.queue.put(srpc_pb2.AdapterRegistRequest(bot_id=echo, platform=value))

    async def receive(self, message):
        await get_async_stub().AdapterReceive(
            srpc_pb2.AdapterRequest(
                platform=self.platform,
                bot_id=self.bot_id,
                value=json.dumps(message or {}, ensure_ascii=False),
            ),
            metadata=metadata,
        )

    async def push(self, message):
        response = await get_async_stub().AdapterPush(
            srpc_pb2.AdapterRequest(
                platform=self.platform,
                bot_id=self.bot_id,
                value=json.dumps(message or {}, ensure_ascii=False),
            ),
            metadata=metadata,
        )
        return response.value or ""

    async def destroy(self):
        if self.queue is not None:
            await self.queue.put(None)

    async def sender(self, options=None):
        response = await get_async_stub().AdapterSender(
            srpc_pb2.AdapterRequest(
                platform=self.platform,
                bot_id=self.bot_id,
                value=json.dumps(options or {}, ensure_ascii=False),
            ),
            metadata=metadata,
        )
        return Sender(response.value) if response.value else None


class Utils:
    def buildCQTag(self, cq_type, params=None, prefix="CQ"):
        params = params or {}
        values = ",".join(f"{key}={value}" for key, value in params.items())
        return f"[{prefix}:{cq_type}{',' + values if values else ''}]"

    def parseCQText(self, text, prefix="CQ"):
        result = []
        last = 0
        pattern = re.compile(rf"\[{re.escape(prefix)}:(\w+)(.*?)\]", re.S)
        for match in pattern.finditer(str(text or "")):
            if match.start() > last:
                result.append(text[last : match.start()])
            params = {}
            for key, value in re.findall(r"(\w+)=([^,]+)", match.group(2)):
                params[key] = value.strip()
            result.append({"type": match.group(1), "params": params})
            last = match.end()
        if last < len(str(text or "")):
            result.append(str(text or "")[last:])
        return result

    def image(self, url):
        return self.buildCQTag("image", {"url": url})

    def video(self, url):
        return self.buildCQTag("video", {"url": url})


utils = Utils()


def _normalize_list(value):
    if value is None:
        return []
    if isinstance(value, list):
        return [str(item).strip() for item in value if str(item).strip()]
    return [item.strip() for item in re.split(r"[,&\s]+", str(value)) if item.strip()]


async def pushAdmin(content, options=None):
    options = options or {}
    result = []
    platforms = _normalize_list(options.get("platform")) + _normalize_list(options.get("platforms"))
    platforms = list(dict.fromkeys(platforms or await Bucket("sillyGirl").buckets()))
    bot_id = str(options.get("botId") or options.get("bot_id") or "")
    explicit_users = _normalize_list(options.get("userIds")) + _normalize_list(options.get("users"))
    explicit_users = list(dict.fromkeys(explicit_users))
    for platform in platforms:
        users = explicit_users or _normalize_list(await Bucket(platform).get("masters", ""))
        adapter = Adapter(platform, bot_id)
        for user_id in users:
            try:
                message_id = await adapter.push({"user_id": user_id, "content": content})
                result.append({"platform": platform, "bot_id": bot_id, "user_id": user_id, "message_id": message_id})
            except Exception as exc:
                result.append({"platform": platform, "bot_id": bot_id, "user_id": user_id, "error": str(exc)})
    return result


async def sleep(ms=1000):
    await asyncio.sleep(float(ms or 0) / 1000)


async def restart():
    return await Bucket("sillyGirl").set("started_at", time.strftime("%Y-%m-%d %H:%M:%S"))


def _compact_runtime_output(value):
    text = str(value or "").strip()
    if len(text) > 2000:
        return text[:2000] + "..."
    return text


def _run_process(cwd, args, timeout=120):
    proc = subprocess.run(
        args,
        cwd=cwd,
        text=True,
        capture_output=True,
        timeout=max(10, min(int(timeout or 120), 600)),
        check=False,
    )
    if proc.returncode != 0:
        message = _compact_runtime_output(proc.stderr or proc.stdout)
        raise RuntimeError(message or f"{args[0]} 执行失败：exit {proc.returncode}")
    return {
        "stdout": proc.stdout or "",
        "stderr": proc.stderr or "",
    }


def _add_repo_candidate(candidates, value):
    path = str(value or "").strip()
    if not path:
        return
    path = os.path.abspath(path)
    if path not in candidates:
        candidates.append(path)


def _is_sillygirl_repo(path, timeout):
    if not os.path.isdir(path):
        return False
    try:
        _run_process(path, ["git", "rev-parse", "--is-inside-work-tree"], timeout)
        remote = _run_process(path, ["git", "config", "--get", "remote.origin.url"], timeout)["stdout"]
        remote = remote.strip().lower()
        return "sillygirl" in remote and "sillygirl_plugins" not in remote and "sillygirl-plugins" not in remote
    except Exception:
        return False


def _resolve_sillygirl_repo(configured=None, timeout=120):
    candidates = []
    _add_repo_candidate(candidates, configured)
    for env_key in ("SILLYGIRL_APP_DIR", "APP_HOME", "HOME"):
        _add_repo_candidate(candidates, os.environ.get(env_key))
    _add_repo_candidate(candidates, os.getcwd())
    _add_repo_candidate(candidates, "/app")
    _add_repo_candidate(candidates, "/data/sillyGirl")

    for path in list(candidates):
        parent = os.path.abspath(os.path.join(path, os.pardir))
        _add_repo_candidate(candidates, parent)

    for path in candidates:
        if _is_sillygirl_repo(path, timeout):
            return path
    raise RuntimeError("未找到可更新的 SillyGirl Git 仓库")


def _current_branch(repo, timeout):
    branch = _run_process(repo, ["git", "rev-parse", "--abbrev-ref", "HEAD"], timeout)["stdout"].strip()
    if not branch or branch == "HEAD":
        raise RuntimeError("当前仓库处于 detached HEAD，请显式指定 branch")
    return branch


def _pull_args(repo, remote="origin", branch=None, timeout=120):
    remote = str(remote or "origin").strip() or "origin"
    branch = str(branch or "").strip() or _current_branch(repo, timeout)
    upstream = f"{remote}/{branch}"
    _run_process(repo, ["git", "rev-parse", "--verify", upstream], timeout)
    return ["git", "pull", "--ff-only", remote, branch]


async def update(options=None):
    try:
        options = options or {}
        if isinstance(options, str):
            options = {"appDir": options}
        if not isinstance(options, dict):
            raise RuntimeError("update options 必须是 dict 或 appDir 字符串")
        timeout = max(10, min(int(options.get("timeout") or 120), 600))
        remote = str(options.get("gitRemote") or "origin").strip() or "origin"
        repo = await asyncio.to_thread(_resolve_sillygirl_repo, options.get("appDir"), timeout)
        before = (
            await asyncio.to_thread(_run_process, repo, ["git", "rev-parse", "--short", "HEAD"], timeout)
        )["stdout"].strip()
        await asyncio.to_thread(_run_process, repo, ["git", "fetch", remote, "--prune"], timeout)
        pull_args = await asyncio.to_thread(_pull_args, repo, remote, options.get("gitBranch"), timeout)
        pull = await asyncio.to_thread(
            _run_process,
            repo,
            pull_args,
            timeout,
        )
        after = (
            await asyncio.to_thread(_run_process, repo, ["git", "rev-parse", "--short", "HEAD"], timeout)
        )["stdout"].strip()
        restarted = bool(options.get("restart"))
        if restarted:
            await restart()
        return {
            "status": True,
            "message": "更新完成",
            "data": {
                "mode": "git",
                "repo": repo,
                "before": before,
                "after": after,
                "changed": before != after,
                "output": _compact_runtime_output(pull.get("stdout") or pull.get("stderr")),
                "restarted": restarted,
            },
        }
    except Exception as exc:
        return {"status": False, "message": str(exc), "data": None}


class Console:
    def __init__(self, plugin_id_value):
        self.plugin_id = plugin_id_value

    def log(self, *args):
        self.send_console_request("log", *args)

    def info(self, *args):
        self.send_console_request("info", *args)

    def error(self, *args):
        self.send_console_request("error", *args)

    def debug(self, *args):
        self.send_console_request("debug", *args)

    def send_console_request(self, console_type, *args):
        content = " ".join(map(str, args))
        request = srpc_pb2.ConsoleRequest(type=console_type, content=content, plugin_id=self.plugin_id)
        try:
            get_stub().Console(request, metadata=metadata)
        except Exception:
            print(content)


console = Console(plugin_id)
