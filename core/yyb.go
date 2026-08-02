package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/smallfawn/sillyGirl/utils"
)

const yybPanelsStorageKey = "yyb_panels"

var legacyYybPanels = MakeBucket("yyb_panels")

type YybPanel struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Address       string `json:"address"`
	CreatedAt     int    `json:"created_at"`
	UpdatedAt     int    `json:"updated_at"`
	LastCheckedAt int    `json:"last_checked_at"`
	Status        string `json:"status"`
	Message       string `json:"message"`
	AccountCount  int    `json:"account_count"`
}

type yybEnvelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

func init() {
	GinApi(GET, "/api/admin/yyb/panels", RequireAuth, func(ctx *gin.Context) {
		panels := getYybPanels()
		refreshYybPanelsStatus(panels)
		ApiList(ctx, panels, len(panels))
	})

	GinApi(POST, "/api/admin/yyb/panel/test", RequireAuth, func(ctx *gin.Context) {
		panel := YybPanel{}
		if err := ctx.BindJSON(&panel); err != nil {
			ApiFail(ctx, err.Error())
			return
		}
		if err := validateYybPanelInput(&panel); err != nil {
			ApiFail(ctx, err.Error())
			return
		}
		result, err := testYybPanel(panel)
		if err != nil {
			ApiFail(ctx, err.Error())
			return
		}
		ApiOK(ctx, result)
	})

	GinApi(POST, "/api/admin/yyb/panel", RequireAuth, func(ctx *gin.Context) {
		panel := YybPanel{}
		if err := ctx.BindJSON(&panel); err != nil {
			ApiFail(ctx, err.Error())
			return
		}
		if err := validateYybPanelInput(&panel); err != nil {
			ApiFail(ctx, err.Error())
			return
		}
		result, err := testYybPanel(panel)
		if err != nil {
			ApiFail(ctx, err.Error())
			return
		}
		now := int(time.Now().Unix())
		panels := getYybPanels()
		index := -1
		if panel.ID != "" {
			for i := range panels {
				if panels[i].ID == panel.ID {
					index = i
					break
				}
			}
		}
		if panel.ID == "" {
			panel.ID = utils.GenUUID()
			panel.CreatedAt = now
		} else if index >= 0 {
			if panels[index].CreatedAt != 0 {
				panel.CreatedAt = panels[index].CreatedAt
			} else {
				panel.CreatedAt = now
			}
		} else {
			panel.CreatedAt = now
		}
		if panel.Name == "" {
			panel.Name = panel.Address
		}
		panel.UpdatedAt = now
		panel.LastCheckedAt = now
		panel.Status = "online"
		panel.Message = result.Message
		panel.AccountCount = result.AccountCount
		if index >= 0 {
			panels[index] = panel
		} else {
			panels = append(panels, panel)
		}
		saveYybPanels(panels)
		ApiOK(ctx, panel)
	})

	GinApi(DELETE, "/api/admin/yyb/panel", RequireAuth, func(ctx *gin.Context) {
		panel := YybPanel{}
		if err := ctx.BindJSON(&panel); err != nil {
			ApiFail(ctx, err.Error())
			return
		}
		if panel.ID == "" {
			ApiFail(ctx, "缺少 yyb-go ID")
			return
		}
		panels := getYybPanels()
		next := make([]YybPanel, 0, len(panels))
		for _, item := range panels {
			if item.ID != panel.ID {
				next = append(next, item)
			}
		}
		saveYybPanels(next)
		ApiOK(ctx, nil)
	})
}

func getYybPanels() []YybPanel {
	raw := strings.TrimSpace(sillyGirl.GetString(yybPanelsStorageKey))
	if raw != "" {
		panels := []YybPanel{}
		if json.Unmarshal([]byte(strings.TrimPrefix(raw, "o:")), &panels) == nil {
			return panels
		}
	}
	panels := getLegacyYybPanels()
	if len(panels) > 0 {
		saveYybPanels(panels)
	}
	return panels
}

func getLegacyYybPanels() []YybPanel {
	panels := []YybPanel{}
	legacyYybPanels.Foreach(func(_, data []byte) error {
		panel := YybPanel{}
		if json.Unmarshal(data, &panel) == nil && panel.ID != "" {
			panels = append(panels, panel)
		}
		return nil
	})
	return panels
}

func saveYybPanels(panels []YybPanel) {
	sillyGirl.Set(yybPanelsStorageKey, utils.JsonMarshal(panels))
}

func validateYybPanelInput(panel *YybPanel) error {
	panel.Name = strings.TrimSpace(panel.Name)
	panel.Address = normalizeYybAddress(panel.Address)
	if panel.Address == "" {
		return errors.New("yyb-go 地址不能为空")
	}
	parsed, err := url.ParseRequestURI(panel.Address)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("yyb-go 地址格式错误：%v", err)
	}
	return nil
}

func normalizeYybAddress(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return ""
	}
	if !strings.HasPrefix(address, "http://") && !strings.HasPrefix(address, "https://") {
		address = "http://" + address
	}
	return strings.TrimRight(address, "/")
}

func refreshYybPanelsStatus(panels []YybPanel) {
	for index := range panels {
		panel := &panels[index]
		panel.LastCheckedAt = int(time.Now().Unix())
		result, err := testYybPanel(*panel)
		if err != nil {
			panel.Status = "offline"
			panel.Message = err.Error()
			continue
		}
		panel.Status = "online"
		panel.Message = result.Message
		panel.AccountCount = result.AccountCount
	}
}

func testYybPanel(panel YybPanel) (*YybPanel, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	envelope, err := requestYybJSON(ctx, panel.Address, "/health", nil)
	if err != nil {
		return nil, err
	}
	if envelope.Code != 0 {
		return nil, fmt.Errorf("yyb-go 健康检查失败：%s", yybEnvelopeMessage(envelope, "health check failed"))
	}
	panel.Address = normalizeYybAddress(panel.Address)
	panel.Status = "online"
	panel.Message = "连接正常"
	panel.LastCheckedAt = int(time.Now().Unix())
	panel.AccountCount = countYybAccounts(ctx, panel.Address)
	return &panel, nil
}

func countYybAccounts(ctx context.Context, address string) int {
	envelope, err := requestYybJSON(ctx, address, "/accounts", nil)
	if err != nil {
		return 0
	}
	var accounts []json.RawMessage
	if err := json.Unmarshal(envelope.Data, &accounts); err != nil {
		return 0
	}
	return len(accounts)
}

func requestYybJSON(ctx context.Context, address string, path string, query map[string]string) (*yybEnvelope, error) {
	address = normalizeYybAddress(address)
	values := url.Values{}
	for key, value := range query {
		values.Set(key, value)
	}
	requestURL := address + path
	if encoded := values.Encode(); encoded != "" {
		requestURL += "?" + encoded
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("yyb-go 请求失败：%v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(raw))
		if len(message) > 200 {
			message = message[:200]
		}
		return nil, fmt.Errorf("yyb-go HTTP %d：%s", resp.StatusCode, message)
	}
	envelope := &yybEnvelope{}
	if err := json.Unmarshal(raw, envelope); err != nil {
		message := strings.TrimSpace(string(raw))
		if len(message) > 200 {
			message = message[:200]
		}
		return nil, fmt.Errorf("yyb-go 返回非 JSON：%s", message)
	}
	return envelope, nil
}

func yybEnvelopeMessage(envelope *yybEnvelope, fallback string) string {
	if envelope == nil {
		return fallback
	}
	if strings.TrimSpace(envelope.Msg) != "" {
		return envelope.Msg
	}
	return fallback
}
