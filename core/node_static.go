package core

import (
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

var statics sync.Map

func addStatic(uuid, path string) {
	statics.Store(uuid, path)
}

func remStatic(uuid string) {
	statics.Delete(uuid)
}

func FindFile(c *gin.Context) {
	// 获取文件名
	filename := c.Param("filename")

	served := false
	statics.Range(func(_, value any) bool {
		staticRoot, ok := value.(string)
		if !ok {
			return true
		}
		filePath, err := safeStaticFilePath(staticRoot, filename)
		if err != nil {
			return true
		}
		// 判断文件是否存在
		info, err := os.Stat(filePath)
		if err == nil && !info.IsDir() {
			served = true
			c.File(filePath)
			return false
		} else if err != nil {
			console.Log(err)
		}
		return true
	})
	// 如果文件不存在，返回404错误
	if !served && !c.Writer.Written() {
		c.AbortWithStatus(http.StatusNotFound)
	}
}

func safeStaticFilePath(staticRoot string, filename string) (string, error) {
	staticRoot = strings.TrimSpace(staticRoot)
	filename = strings.TrimSpace(filename)
	if staticRoot == "" || filename == "" {
		return "", errors.New("static root or filename is empty")
	}
	normalized := strings.ReplaceAll(filename, "\\", "/")
	if strings.Contains(normalized, "\x00") || strings.Contains(normalized, ":") || strings.HasPrefix(normalized, "/") {
		return "", errors.New("invalid static filename")
	}
	for _, segment := range strings.Split(normalized, "/") {
		if segment == ".." {
			return "", errors.New("static filename escapes root")
		}
	}
	cleanName := path.Clean(normalized)
	if cleanName == "." || cleanName == ".." || strings.HasPrefix(cleanName, "../") {
		return "", errors.New("invalid static filename")
	}
	rootAbs, err := filepath.Abs(filepath.Clean(staticRoot))
	if err != nil {
		return "", err
	}
	targetAbs, err := filepath.Abs(filepath.Clean(filepath.Join(rootAbs, filepath.FromSlash(cleanName))))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil || rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("static filename escapes root")
	}
	return targetAbs, nil
}

// Server.GET("/api/file/:filename", FindFile)
// 	Server.GET("/api/decode/:random", Base642Binary)

func Base642Binary(c *gin.Context) {
	random := c.Param("random")
	s, ok := temp.Get("base64_" + random).(string)
	if !ok {
		c.String(http.StatusBadRequest, "Invalid input")
		return
	}
	input := strings.TrimPrefix(s, "base64://")
	data, err := base64.StdEncoding.DecodeString(input)
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid input")
		return
	}
	// 解析图片格式
	_, format, err := image.DecodeConfig(strings.NewReader(string(data)))
	fmt.Println(format, err)
	if err != nil {
		c.Header("Content-Type", "application/octet-stream")
	} else {
		// 根据图片格式设置响应头
		switch format {
		case "jpeg":
			c.Header("Content-Type", "image/jpeg")
		case "png":
			c.Header("Content-Type", "image/png")
		default:
			c.Header("Content-Type", "application/octet-stream")
			return
		}
	}
	c.Data(http.StatusOK, "application/octet-stream", data)
}
