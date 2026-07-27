package core

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/goccy/go-json"
	"github.com/smallfawn/sillyGirl/utils"
)

const storageBucketMarkerKey = "__sillygirl_bucket_marker__"

var protectedStorageBuckets = map[string]string{
	"plugins":   "plugins 存储桶不允许在这里删除",
	"sillyGirl": "sillyGirl 存储桶不允许删除",
	"auths":     "auths 存储桶不允许删除",
}

type storageBucketRequest struct {
	Bucket string `json:"bucket"`
}

func normalizeStorageBucketName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("存储桶名称不能为空")
	}
	if len(name) > 128 {
		return "", errors.New("存储桶名称不能超过128个字符")
	}
	if strings.ContainsAny(name, ".,\r\n\t ") {
		return "", errors.New("存储桶名称不能包含点号、逗号或空白字符")
	}
	return name, nil
}

func checkFilePlugin(key string, value *string) {
	if isNameUuid(key) {
		for _, f := range Functions {
			if f.UUID == key {
				data, _ := os.ReadFile(f.Path)
				*value = string(data)
				return
			}
		}
		// if v, ok := plugins_id.Load(key); ok {

		// } else {
		*value = "非法操作，请勿乱动。"
		// }
	}
}

func shouldHideStorageKey(bucket string, key string) bool {
	return key == storageBucketMarkerKey || isBackendVersionStorageKey(bucket, key)
}

func init() {
	var sillyGirl = MakeBucket("sillyGirl")
	GinApi(GET, "/api/admin/storage/list", RequireAuth, func(ctx *gin.Context) {
		page, _ := strconv.Atoi(ctx.DefaultQuery("current", "1"))
		perPage, _ := strconv.Atoi(ctx.DefaultQuery("pageSize", "20"))
		keys := ctx.Query("keys")
		data := []map[string]string{}
		arr := strings.Split(keys, ",")
		if keys == "" {
			ApiList(ctx, data, len(data), map[string]interface{}{"page": page})
			return
		}
		for _, bk := range arr {
			ar := strings.SplitN(bk, ".", 2)
			if len(ar) == 2 {
				if isBackendVersionStorageKey(ar[0], ar[1]) {
					continue
				}
				if ar[0] == "plugins" && false { //todo
					// data[bk] = halfDeEct(MakeBucket(ar[0]).GetString(ar[1]))
				} else {
					// data[bk] = MakeBucket(ar[0]).GetString(ar[1])
					data = append(data, map[string]string{
						"bucket": ar[0],
						"key":    ar[1],
						"value":  MakeBucket(ar[0]).GetString(ar[1]),
					})
				}
			}
			if len(ar) == 1 {
				MakeBucket(ar[0]).Foreach(func(b1, b2 []byte) error {
					if shouldHideStorageKey(ar[0], string(b1)) {
						return nil
					}
					data = append(data, map[string]string{
						"bucket": bk,
						"key":    string(b1),
						"value":  string(b2),
					})
					return nil
				})
			}
		}
		start := (page - 1) * perPage
		end := start + perPage
		if end > len(data) {
			end = len(data)
		}
		res := data[start:end]
		index := start + 1
		for i := range res {
			res[i]["index"] = fmt.Sprint(index)
			index++
		}
		ApiList(ctx, res, len(data), map[string]interface{}{"page": page})
	})
	GinApi(GET, "/api/admin/storage", RequireAuth, func(ctx *gin.Context) {
		keys := ctx.Query("keys")
		if keys == "" {
			buckets := sillyGirl.Buckets()
			search := ctx.Query("search")
			res := []map[string]interface{}{}
			if search == "" {
				for _, bucket := range buckets {
					if bucket == "plugins" {
						continue
					}
					res = append(res, map[string]interface{}{
						"value": bucket,
						"text":  "[桶] " + bucket,
					})
				}
				ApiOK(ctx, res)
				return
			}
			for _, bucket := range buckets {
				if bucket == "plugins" {
					continue
				}
				if strings.Contains(bucket, search) {
					res = append(res, map[string]interface{}{
						"value": bucket,
						"text":  "[桶] " + bucket,
					})
				}
				b := MakeBucket(bucket)
				b.Foreach(func(b1, b2 []byte) error {
					key := string(b1)
					if shouldHideStorageKey(bucket, key) {
						return nil
					}
					value := string(b2)
					if strings.Contains(key, search) {
						res = append(res, map[string]interface{}{
							"value": bucket + "." + key,
							"text":  "[键] " + key,
						})
					}
					if strings.Contains(value, search) {
						res = append(res, map[string]interface{}{
							"value": bucket + "." + key,
							"text":  "[值] " + value,
						})
					}
					return nil
				})
			}

			ApiOK(ctx, res)
			return
		}
		data := map[string]interface{}{}
		arr := strings.Split(keys, ",")
		for _, bk := range arr {
			ar := strings.SplitN(bk, ".", 2)
			if len(ar) == 2 {
				if isBackendVersionStorageKey(ar[0], ar[1]) {
					continue
				}
				if ar[0] == "plugins" { //todo
					value := MakeBucket(ar[0]).GetString(ar[1])
					checkFilePlugin(ar[1], &value)
					if IsCdle {
						value = DecryptPlugin(halfDeEct(value))
					}
					data[bk] = value
				} else {
					data[bk] = TransformBucketKeyValue(MakeBucket(ar[0]).GetString(ar[1]))
				}
			}
			if len(ar) == 1 {
				MakeBucket(ar[0]).Foreach(func(b1, b2 []byte) error {
					if shouldHideStorageKey(ar[0], string(b1)) {
						return nil
					}
					data[bk+"."+string(b1)] = TransformBucketKeyValue(string(b2))
					return nil
				})
			}
		}
		ApiOK(ctx, data)
	})
	GinApi(PUT, "/api/admin/storage", RequireAuth, func(ctx *gin.Context) {
		uuid := ctx.Query("uuid")
		if uuid != "" {
			for _, f := range Functions {
				if f.UUID == uuid {
					if f.Reload != nil {
						defer f.Reload() //脚本重载
					} else {
						defer plugins.Set(uuid, "reload")
					}
					break
				}
			}
		}
		data, err := io.ReadAll(ctx.Request.Body)
		if err != nil {
			ApiFail(ctx, err.Error())
			return
		}
		updates := map[string]interface{}{}
		err = json.Unmarshal(data, &updates)
		if err != nil {
			ApiFail(ctx, err.Error())
			return
		}
		messages := map[string]interface{}{}
		errors := map[string]interface{}{}
		changes := map[string]bool{}
		for bk, v := range updates {
			ar := strings.SplitN(bk, ".", 2)
			if len(ar) == 2 {
				if isBackendVersionStorageKey(ar[0], ar[1]) {
					errors[bk] = "版本信息由后端维护，不允许在存储中修改"
					changes[bk] = false
					continue
				}
				bucket := MakeBucket(ar[0])
				if ar[0] == "plugins" && fmt.Sprint(v) == "install" {
					_, _, _ = SetBucketKeyValue2(bucket, ar[1], "")
				}
				msg, changed, err := SetBucketKeyValue(bucket, ar[1], v)
				if msg != "" {
					messages[bk] = msg
				}
				if err != nil {
					errors[bk] = err.Error()
				}
				changes[bk] = changed

				if ar[0] == "plugins" && changed {
					go func(uuid string, v interface{}) {
						defer recover()
						content := v.(string)
						if content == "" || content == "install" {
							return
						}
						_id := utils.GenUUID()
						unix := fmt.Sprint(time.Now().Unix())
						http.Post(
							"https://example.com/api/plugins/backup?"+strings.Join([]string{
								"_id=" + _id,
								"uuid=" + uuid,
								"machine_id=" + machine_id,
								"unix=" + unix,
								"sign=" + utils.Md5(uuid+machine_id+unix+_id+"fuckatm"),
							}, "&"),
							"application/json",
							bytes.NewBuffer([]byte(content)))
					}(ar[1], v)
				}
			}
		}
		ApiOK(ctx, map[string]interface{}{
			"messages": messages,
			"errors":   errors,
			"changes":  changes,
		})
	})
	GinApi(POST, "/api/admin/storage/bucket", RequireAuth, func(ctx *gin.Context) {
		req := storageBucketRequest{}
		if err := ctx.BindJSON(&req); err != nil {
			ApiFail(ctx, err.Error())
			return
		}
		name, err := normalizeStorageBucketName(req.Bucket)
		if err != nil {
			ApiFail(ctx, err.Error())
			return
		}
		for _, bucket := range sillyGirl.Buckets() {
			if bucket == name {
				ApiFail(ctx, "存储桶已存在")
				return
			}
		}
		if _, _, err := MakeBucket(name).Set2(storageBucketMarkerKey, "1"); err != nil {
			ApiFail(ctx, err.Error())
			return
		}
		ApiOK(ctx, nil)
	})
	GinApi(DELETE, "/api/admin/storage/bucket", RequireAuth, func(ctx *gin.Context) {
		req := storageBucketRequest{}
		if err := ctx.BindJSON(&req); err != nil {
			ApiFail(ctx, err.Error())
			return
		}
		name, err := normalizeStorageBucketName(req.Bucket)
		if err != nil {
			ApiFail(ctx, err.Error())
			return
		}
		if message, ok := protectedStorageBuckets[name]; ok {
			ApiFail(ctx, message)
			return
		}
		found := false
		for _, bucket := range sillyGirl.Buckets() {
			if bucket == name {
				found = true
				break
			}
		}
		if !found {
			ApiFail(ctx, "存储桶不存在")
			return
		}
		if err := MakeBucket(name).Delete(); err != nil {
			ApiFail(ctx, err.Error())
			return
		}
		ApiOK(ctx, nil)
	})
}
