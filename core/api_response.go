package core

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func ApiOK(ctx *gin.Context, data interface{}) {
	ctx.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "成功",
		"data":    data,
	})
}

func ApiList(ctx *gin.Context, list interface{}, total int, extras ...map[string]interface{}) {
	data := gin.H{
		"list":  list,
		"total": total,
	}
	for _, extra := range extras {
		for key, value := range extra {
			data[key] = value
		}
	}
	ApiOK(ctx, data)
}

func ApiFail(ctx *gin.Context, message string) {
	ApiError(ctx, http.StatusOK, message)
}

func ApiError(ctx *gin.Context, httpStatus int, message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "请求失败"
	}
	if httpStatus == 0 {
		httpStatus = http.StatusOK
	}
	ctx.JSON(httpStatus, gin.H{
		"status":  false,
		"message": message,
		"data":    nil,
	})
}
