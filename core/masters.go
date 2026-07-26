package core

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/smallfawn/sillyGirl/utils"
)

type Master struct {
	Platform string `json:"platform"`
	Nickname string `json:"nickname"`
	ID       string `json:"number"`
	Index    int    `json:"id"`
	Unix     int    `json:"unix"`
}

func init() {
	GinApi(GET, "/api/admin/master/list", RequireAuth, func(c *gin.Context) {
		plts := getPltsArray()
		ms := []Master{}
		i := 1
		for _, plt := range plts {
			v := MakeBucket(plt)
			masters := strings.Split(v.GetString("masters"), "&")
			for _, master := range masters {
				if master == "" {
					continue
				}
				nk := Nickname{ID: master}
				nickname.First(&nk)
				ms = append(ms, Master{
					Platform: plt,
					Nickname: nk.Value,
					ID:       master,
					Index:    i,
					Unix:     nk.Unix,
				})
				i++
			}
		}
		ApiOK(c, map[string]interface{}{
			"list":      ms,
			"platforms": getPltsLabel(),
		})
	})
	GinApi(POST, "/api/admin/master", RequireAuth, func(c *gin.Context) {
		m := Master{}
		c.BindJSON(&m)
		if m.ID == "" {
			ApiFail(c, "缺少号码字段")
			return
		}
		if m.Platform == "" {
			nk := Nickname{ID: m.ID}
			nickname.First(&nk)
			if nk.Platform != "" {
				m.Platform = nk.Platform
			}
		}
		if m.Platform == "" {
			ApiFail(c, "缺少平台字段")
			return
		}
		v := MakeBucket(m.Platform)
		masters := strings.Split(v.GetString("masters"), "&")
		v.Set("masters", strings.Join(utils.Unique(masters, m.ID), "&"))

		ApiOK(c, nil)
	})
	GinApi(DELETE, "/api/admin/master", RequireAuth, func(c *gin.Context) {
		m := Master{}
		c.BindJSON(&m)
		if m.ID == "" {
			ApiFail(c, "缺少账号字段")
			return
		}
		if m.Platform == "" {
			nk := Nickname{ID: m.ID}
			nickname.First(&nk)
			if nk.Platform != "" {
				m.Platform = nk.Platform
			}
		}
		if m.Platform == "" {
			ApiFail(c, "缺少平台字段")
			return
		}
		v := MakeBucket(m.Platform)
		masters := strings.Split(v.GetString("masters"), "&")
		v.Set("masters", strings.Join(utils.Remove(masters, m.ID), "&"))

		ApiOK(c, nil)
	})
}
