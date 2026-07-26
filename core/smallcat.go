package core

import (
	"bytes"
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

const smallcatPanelsStorageKey = "smallcat_panels"

var legacySmallcatPanels = MakeBucket("smallcat_panels")

type SmallcatPanel struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Address       string `json:"address"`
	APIAuth       string `json:"api_auth"`
	CreatedAt     int    `json:"created_at"`
	UpdatedAt     int    `json:"updated_at"`
	LastCheckedAt int    `json:"last_checked_at"`
	Status        string `json:"status"`
	Message       string `json:"message"`
}

type smallcatAuthValidateResponse struct {
	Status  bool            `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type PublicSmallcatPanel struct {
	Index   int    `json:"index"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func init() {
	GinApi(GET, "/api/admin/smallcat/panels", RequireAuth, func(ctx *gin.Context) {
		panels := getSmallcatPanels()
		ApiList(ctx, panels, len(panels))
	})

	GinApi(POST, "/api/admin/smallcat/panel/test", RequireAuth, func(ctx *gin.Context) {
		panel := SmallcatPanel{}
		if err := ctx.BindJSON(&panel); err != nil {
			ApiFail(ctx, err.Error())
			return
		}
		if err := validateSmallcatPanelInput(&panel); err != nil {
			ApiFail(ctx, err.Error())
			return
		}
		result, err := testSmallcatPanel(panel)
		if err != nil {
			ApiFail(ctx, err.Error())
			return
		}
		ApiOK(ctx, result)
	})

	GinApi(POST, "/api/admin/smallcat/panel", RequireAuth, func(ctx *gin.Context) {
		panel := SmallcatPanel{}
		if err := ctx.BindJSON(&panel); err != nil {
			ApiFail(ctx, err.Error())
			return
		}
		if err := validateSmallcatPanelInput(&panel); err != nil {
			ApiFail(ctx, err.Error())
			return
		}
		result, err := testSmallcatPanel(panel)
		if err != nil {
			ApiFail(ctx, err.Error())
			return
		}
		now := int(time.Now().Unix())
		panels := getSmallcatPanels()
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
		if index >= 0 {
			panels[index] = panel
		} else {
			panels = append(panels, panel)
		}
		saveSmallcatPanels(panels)
		ApiOK(ctx, panel)
	})

	GinApi(DELETE, "/api/admin/smallcat/panel", RequireAuth, func(ctx *gin.Context) {
		panel := SmallcatPanel{}
		if err := ctx.BindJSON(&panel); err != nil {
			ApiFail(ctx, err.Error())
			return
		}
		if panel.ID == "" {
			ApiFail(ctx, "缺少 smallcat ID")
			return
		}
		panels := getSmallcatPanels()
		next := make([]SmallcatPanel, 0, len(panels))
		for _, item := range panels {
			if item.ID != panel.ID {
				next = append(next, item)
			}
		}
		saveSmallcatPanels(next)
		ApiOK(ctx, nil)
	})
}

func getSmallcatPanels() []SmallcatPanel {
	raw := strings.TrimSpace(sillyGirl.GetString(smallcatPanelsStorageKey))
	if raw != "" {
		panels := []SmallcatPanel{}
		if json.Unmarshal([]byte(strings.TrimPrefix(raw, "o:")), &panels) == nil {
			return panels
		}
	}
	panels := getLegacySmallcatPanels()
	if len(panels) > 0 {
		saveSmallcatPanels(panels)
	}
	return panels
}

func getLegacySmallcatPanels() []SmallcatPanel {
	panels := []SmallcatPanel{}
	legacySmallcatPanels.Foreach(func(_, data []byte) error {
		panel := SmallcatPanel{}
		if json.Unmarshal(data, &panel) == nil && panel.ID != "" {
			panels = append(panels, panel)
		}
		return nil
	})
	return panels
}

func saveSmallcatPanels(panels []SmallcatPanel) {
	sillyGirl.Set(smallcatPanelsStorageKey, utils.JsonMarshal(panels))
}

func validateSmallcatPanelInput(panel *SmallcatPanel) error {
	panel.Name = strings.TrimSpace(panel.Name)
	panel.Address = normalizeSmallcatAddress(panel.Address)
	panel.APIAuth = strings.TrimSpace(panel.APIAuth)
	if panel.Address == "" {
		return errors.New("smallcat 地址不能为空")
	}
	if panel.APIAuth == "" {
		return errors.New("api_auth 不能为空")
	}
	parsed, err := url.ParseRequestURI(panel.Address)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("smallcat 地址格式错误：%v", err)
	}
	return nil
}

func normalizeSmallcatAddress(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return ""
	}
	if !strings.HasPrefix(address, "http://") && !strings.HasPrefix(address, "https://") {
		address = "http://" + address
	}
	return strings.TrimRight(address, "/")
}

func testSmallcatPanel(panel SmallcatPanel) (*SmallcatPanel, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, panel.Address+"/api/auth/validate", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("auth", panel.APIAuth)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("smallcat 接口连接失败：%v", err)
	}
	defer resp.Body.Close()
	authResp := smallcatAuthValidateResponse{}
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return nil, fmt.Errorf("smallcat 接口返回无法解析：%v", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("smallcat 接口 HTTP %d：%s", resp.StatusCode, authResp.Message)
	}
	if !authResp.Status {
		if authResp.Message == "" {
			authResp.Message = "验证失败，请检查 API AUTH"
		}
		return nil, errors.New(authResp.Message)
	}
	panel.Address = normalizeSmallcatAddress(panel.Address)
	panel.Status = "online"
	panel.Message = "验证通过"
	panel.LastCheckedAt = int(time.Now().Unix())
	return &panel, nil
}

func publicSmallcatPanels() []PublicSmallcatPanel {
	panels := getSmallcatPanels()
	result := make([]PublicSmallcatPanel, 0, len(panels))
	for index, panel := range panels {
		result = append(result, PublicSmallcatPanel{
			Index:   index + 1,
			ID:      panel.ID,
			Name:    firstNonEmpty(panel.Name, fmt.Sprintf("smallcat #%d", index+1)),
			Status:  panel.Status,
			Message: panel.Message,
		})
	}
	return result
}

func smallcatPanelByIndex(index int) (*SmallcatPanel, error) {
	panels := getSmallcatPanels()
	if len(panels) == 0 {
		return nil, errors.New("后台未绑定 smallcat")
	}
	if index <= 0 {
		index = 1
	}
	if index > len(panels) {
		return nil, fmt.Errorf("smallcat 编号 %d 不存在", index)
	}
	panel := panels[index-1]
	if panel.Address == "" || panel.APIAuth == "" {
		return nil, errors.New("smallcat 配置不完整")
	}
	return &panel, nil
}

func requestSmallcatJSON(panel *SmallcatPanel, method string, path string, body interface{}, query map[string]string) (json.RawMessage, error) {
	if panel == nil {
		return nil, errors.New("smallcat 配置不存在")
	}
	address := normalizeSmallcatAddress(panel.Address)
	values := url.Values{}
	for key, value := range query {
		values.Set(key, value)
	}
	requestURL := address + path
	if encoded := values.Encode(); encoded != "" {
		requestURL += "?" + encoded
	}
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("auth", panel.APIAuth)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("smallcat 请求失败：%v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(raw))
		if len(message) > 200 {
			message = message[:200]
		}
		return nil, fmt.Errorf("smallcat HTTP %d：%s", resp.StatusCode, message)
	}
	if !json.Valid(raw) {
		message := strings.TrimSpace(string(raw))
		if len(message) > 200 {
			message = message[:200]
		}
		return nil, fmt.Errorf("smallcat 返回非 JSON：%s", message)
	}
	return json.RawMessage(raw), nil
}
