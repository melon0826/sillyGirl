package clawbot

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/smallfawn/sillyGirl/core"
	"github.com/smallfawn/sillyGirl/core/storage"
	"github.com/smallfawn/sillyGirl/utils"
)

const (
	platform               = "clawbot"
	defaultAPIBase         = "https://ilinkai.weixin.qq.com"
	defaultChannelVer      = "2.4.6"
	defaultIlinkAppID      = "bot"
	defaultPollTimeout     = 35 * time.Second
	defaultAPITimeout      = 15 * time.Second
	messageTypeUser        = 1
	messageTypeBot         = 2
	messageItemText        = 1
	messageStateGenerating = 1
	messageStateFinish     = 2
)

var clawbot = core.MakeBucket(platform)

var runtime = struct {
	sync.Mutex
	cancel context.CancelFunc
}{}

var (
	compactNewlinePattern = regexp.MustCompile(`[\r\n]+`)
	cqCodePattern         = regexp.MustCompile(`\[CQ:[^\]]+\]`)
)

type apiClient struct {
	baseURL        string
	token          string
	channelVersion string
	appID          string
	clientVersion  string
	client         *http.Client
	debug          bool
}

type baseInfo struct {
	ChannelVersion string `json:"channel_version,omitempty"`
}

type getUpdatesRequest struct {
	GetUpdatesBuf string   `json:"get_updates_buf"`
	BaseInfo      baseInfo `json:"base_info"`
}

type getUpdatesResponse struct {
	Ret                int             `json:"ret,omitempty"`
	ErrCode            int             `json:"errcode,omitempty"`
	ErrMsg             string          `json:"errmsg,omitempty"`
	Messages           []weixinMessage `json:"msgs,omitempty"`
	GetUpdatesBuf      string          `json:"get_updates_buf,omitempty"`
	LongPollingTimeout int             `json:"longpolling_timeout_ms,omitempty"`
}

type sendMessageRequest struct {
	Message  weixinMessage `json:"msg"`
	BaseInfo baseInfo      `json:"base_info"`
}

type sendMessageResponse struct {
	Ret    int    `json:"ret,omitempty"`
	ErrMsg string `json:"errmsg,omitempty"`
}

type weixinMessage struct {
	Seq          int64         `json:"seq,omitempty"`
	MessageID    int64         `json:"message_id,omitempty"`
	FromUserID   string        `json:"from_user_id,omitempty"`
	ToUserID     string        `json:"to_user_id,omitempty"`
	ClientID     string        `json:"client_id,omitempty"`
	CreateTimeMs int64         `json:"create_time_ms,omitempty"`
	UpdateTimeMs int64         `json:"update_time_ms,omitempty"`
	DeleteTimeMs int64         `json:"delete_time_ms,omitempty"`
	SessionID    string        `json:"session_id,omitempty"`
	GroupID      string        `json:"group_id,omitempty"`
	MessageType  int           `json:"message_type,omitempty"`
	MessageState int           `json:"message_state,omitempty"`
	ItemList     []messageItem `json:"item_list,omitempty"`
	ContextToken string        `json:"context_token,omitempty"`
	RunID        string        `json:"run_id,omitempty"`
}

type messageItem struct {
	Type     int      `json:"type,omitempty"`
	MsgID    string   `json:"msg_id,omitempty"`
	TextItem textItem `json:"text_item,omitempty"`
}

type textItem struct {
	Text string `json:"text,omitempty"`
}

type bot struct {
	api      *apiClient
	adapter  *core.Factory
	botID    string
	syncBuf  string
	pollWait time.Duration
}

func init() {
	for _, key := range []string{"token", "enable", "api_base", "debug", "channel_version"} {
		key := key
		storage.Watch(clawbot, key, func(old, new, key string) *storage.Final {
			go restart()
			return nil
		})
	}
	go func() {
		time.Sleep(2 * time.Second)
		restart()
	}()
}

func restart() {
	runtime.Lock()
	if runtime.cancel != nil {
		runtime.cancel()
		runtime.cancel = nil
	}
	token := strings.TrimSpace(clawbot.GetString("token"))
	if token == "" {
		runtime.Unlock()
		core.Logs.Info("clawbot未启动：未配置 clawbot.token")
		return
	}
	if !enabled() {
		runtime.Unlock()
		core.Logs.Info("clawbot未启动：clawbot.enable=false")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	runtime.cancel = cancel
	runtime.Unlock()

	go run(ctx, token)
}

func run(ctx context.Context, token string) {
	b := &bot{
		api:      newAPIClient(token),
		botID:    "default",
		pollWait: defaultPollTimeout,
	}
	if err := b.start(ctx); err != nil && ctx.Err() == nil {
		core.Logs.Warn("clawbot启动失败：%v", err)
	}
}

func (b *bot) start(ctx context.Context) error {
	b.adapter = &core.Factory{}
	b.adapter.Init(platform, b.botID, nil)
	defer b.adapter.Destroy()
	b.adapter.SetReplyHandler(func(msg map[string]interface{}) string {
		return b.reply(ctx, msg)
	})
	_ = b.api.notifyStart(ctx)
	core.Logs.Info("clawbot长轮询已启动")
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = b.api.notifyStop(stopCtx)
	}()
	return b.poll(ctx)
}

func (b *bot) poll(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		resp, err := b.api.getUpdates(ctx, b.syncBuf, b.pollWait)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			core.Logs.Warn("clawbot获取消息失败：%v", err)
			time.Sleep(3 * time.Second)
			continue
		}
		if resp.GetUpdatesBuf != "" {
			b.syncBuf = resp.GetUpdatesBuf
		}
		if resp.LongPollingTimeout > 0 {
			b.pollWait = time.Duration(resp.LongPollingTimeout)*time.Millisecond + 5*time.Second
		}
		if resp.Ret != 0 || resp.ErrCode != 0 {
			core.Logs.Warn("clawbot接口返回失败：ret=%d errcode=%d errmsg=%s", resp.Ret, resp.ErrCode, resp.ErrMsg)
			time.Sleep(3 * time.Second)
			continue
		}
		for _, item := range resp.Messages {
			b.handleMessage(item)
		}
	}
}

func (b *bot) handleMessage(msg weixinMessage) {
	if msg.MessageType == messageTypeBot || msg.MessageState == messageStateGenerating {
		return
	}
	if msg.MessageType != 0 && msg.MessageType != messageTypeUser {
		return
	}
	content := normalizeText(messageText(msg))
	if content == "" {
		return
	}
	userID := strings.TrimSpace(msg.FromUserID)
	if userID == "" {
		userID = strings.TrimSpace(msg.ToUserID)
	}
	if userID == "" {
		return
	}
	chatID := strings.TrimSpace(msg.GroupID)
	if chatID != "" {
		core.CreateNickName(&core.Nickname{
			Group:    true,
			Value:    chatID,
			ID:       chatID,
			Platform: platform,
			BotsID:   []string{b.botID},
		})
	}
	core.CreateNickName(&core.Nickname{
		Value:    userID,
		ID:       userID,
		Platform: platform,
		BotsID:   []string{b.botID},
	})
	params := map[string]interface{}{
		core.USER_ID:            userID,
		core.CHAT_ID:            core.ChatID(chatID),
		core.CONETNT:            content,
		core.MESSAGE_ID:         messageID(msg),
		"user_name":             userID,
		"chat_name":             chatID,
		"clawbot_context_token": msg.ContextToken,
		"clawbot_to_user_id":    userID,
		"clawbot_from_user_id":  msg.FromUserID,
		"clawbot_session_id":    msg.SessionID,
		"clawbot_run_id":        msg.RunID,
	}
	if b.api.debug {
		core.Logs.Debug("clawbot处理消息：%s", string(utils.JsonMarshal(params)))
	}
	b.adapter.Receive(params)
}

func (b *bot) reply(ctx context.Context, msg map[string]interface{}) string {
	content := normalizeText(stripUnsupportedCQ(stringValue(msg[core.CONETNT])))
	if content == "" {
		return ""
	}
	toUserID := firstNonEmpty(
		stringValue(msg["clawbot_to_user_id"]),
		stringValue(msg[core.USER_ID]),
	)
	contextToken := stringValue(msg["clawbot_context_token"])
	if toUserID == "" || contextToken == "" {
		core.Logs.Warn("clawbot发送消息失败：缺少 to_user_id 或 context_token，ClawBot 仅支持在收到消息上下文内回复")
		return ""
	}
	messageID, err := b.api.sendText(ctx, toUserID, contextToken, stringValue(msg["clawbot_run_id"]), content)
	if err != nil {
		core.Logs.Warn("clawbot发送消息失败：%v", err)
		return ""
	}
	return messageID
}

func newAPIClient(token string) *apiClient {
	return &apiClient{
		baseURL:        strings.TrimRight(firstNonEmpty(clawbot.GetString("api_base"), defaultAPIBase), "/"),
		token:          strings.TrimSpace(token),
		channelVersion: firstNonEmpty(clawbot.GetString("channel_version"), defaultChannelVer),
		appID:          firstNonEmpty(clawbot.GetString("app_id"), defaultIlinkAppID),
		clientVersion:  firstNonEmpty(clawbot.GetString("client_version"), strconv.Itoa(buildClientVersion(defaultChannelVer))),
		client:         &http.Client{},
		debug:          clawbot.GetBool("debug", false),
	}
}

func (a *apiClient) getUpdates(ctx context.Context, syncBuf string, timeout time.Duration) (getUpdatesResponse, error) {
	var resp getUpdatesResponse
	err := a.post(ctx, "ilink/bot/getupdates", getUpdatesRequest{
		GetUpdatesBuf: syncBuf,
		BaseInfo:      a.baseInfo(),
	}, timeout, &resp)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return getUpdatesResponse{GetUpdatesBuf: syncBuf}, nil
		}
		return resp, err
	}
	return resp, nil
}

func (a *apiClient) sendText(ctx context.Context, toUserID string, contextToken string, runID string, text string) (string, error) {
	clientID := fmt.Sprintf("sillygirl-clawbot-%d", time.Now().UnixNano())
	var resp sendMessageResponse
	err := a.post(ctx, "ilink/bot/sendmessage", sendMessageRequest{
		Message: weixinMessage{
			FromUserID:   "",
			ToUserID:     toUserID,
			ClientID:     clientID,
			MessageType:  messageTypeBot,
			MessageState: messageStateFinish,
			ContextToken: contextToken,
			RunID:        runID,
			ItemList: []messageItem{{
				Type: messageItemText,
				TextItem: textItem{
					Text: text,
				},
			}},
		},
		BaseInfo: a.baseInfo(),
	}, defaultAPITimeout, &resp)
	if err != nil {
		return "", err
	}
	if resp.Ret != 0 {
		return "", fmt.Errorf("ret=%d errmsg=%s", resp.Ret, resp.ErrMsg)
	}
	return clientID, nil
}

func (a *apiClient) notifyStart(ctx context.Context) error {
	var resp sendMessageResponse
	return a.post(ctx, "ilink/bot/msg/notifystart", map[string]interface{}{
		"base_info": a.baseInfo(),
	}, 10*time.Second, &resp)
}

func (a *apiClient) notifyStop(ctx context.Context) error {
	var resp sendMessageResponse
	return a.post(ctx, "ilink/bot/msg/notifystop", map[string]interface{}{
		"base_info": a.baseInfo(),
	}, 10*time.Second, &resp)
}

func (a *apiClient) post(ctx context.Context, endpoint string, body interface{}, timeout time.Duration, out interface{}) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	reqCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		reqCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, a.baseURL+"/"+endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	for key, value := range a.headers() {
		req.Header.Set(key, value)
	}
	if a.debug {
		core.Logs.Debug("clawbot请求：POST %s bodyLen=%d", endpoint, len(payload))
	}
	httpResp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()
	data, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return err
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", httpResp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return err
	}
	return nil
}

func (a *apiClient) headers() map[string]string {
	headers := map[string]string{
		"Content-Type":            "application/json",
		"AuthorizationType":       "ilink_bot_token",
		"Authorization":           "Bearer " + a.token,
		"X-WECHAT-UIN":            randomWechatUin(),
		"iLink-App-Id":            a.appID,
		"iLink-App-ClientVersion": a.clientVersion,
	}
	return headers
}

func (a *apiClient) baseInfo() baseInfo {
	return baseInfo{
		ChannelVersion: a.channelVersion,
	}
}

func enabled() bool {
	switch strings.ToLower(strings.TrimSpace(clawbot.GetString("enable"))) {
	case "false", "0", "off", "no":
		return false
	default:
		return true
	}
}

func buildClientVersion(version string) int {
	parts := strings.Split(version, ".")
	values := []int{0, 0, 0}
	for i := 0; i < len(parts) && i < 3; i++ {
		n, _ := strconv.Atoi(parts[i])
		values[i] = n
	}
	return ((values[0] & 0xff) << 16) | ((values[1] & 0xff) << 8) | (values[2] & 0xff)
}

func randomWechatUin() string {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return base64.StdEncoding.EncodeToString([]byte(strconv.FormatInt(time.Now().UnixNano(), 10)))
	}
	value := binary.BigEndian.Uint32(buf[:])
	return base64.StdEncoding.EncodeToString([]byte(strconv.FormatUint(uint64(value), 10)))
}

func messageText(msg weixinMessage) string {
	items := make([]string, 0, len(msg.ItemList))
	for _, item := range msg.ItemList {
		if item.Type == messageItemText && strings.TrimSpace(item.TextItem.Text) != "" {
			items = append(items, item.TextItem.Text)
		}
	}
	return strings.Join(items, "\n")
}

func messageID(msg weixinMessage) string {
	if msg.MessageID != 0 {
		return strconv.FormatInt(msg.MessageID, 10)
	}
	if msg.Seq != 0 {
		return strconv.FormatInt(msg.Seq, 10)
	}
	for _, item := range msg.ItemList {
		if strings.TrimSpace(item.MsgID) != "" {
			return strings.TrimSpace(item.MsgID)
		}
	}
	return ""
}

func normalizeText(value string) string {
	value = strings.ReplaceAll(value, "\\r", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = compactNewlinePattern.ReplaceAllString(value, "\n")
	return strings.TrimSpace(value)
}

func stripUnsupportedCQ(value string) string {
	return strings.TrimSpace(cqCodePattern.ReplaceAllString(value, ""))
}

func stringValue(value interface{}) string {
	return strings.TrimSpace(utils.Itoa(value))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
