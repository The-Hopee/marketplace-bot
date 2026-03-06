package payment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"
)

// ================== Клиент ==================

type TBankClient struct {
	terminalKey     string
	secretKey       string
	baseURL         string
	notificationURL string
	httpClient      *http.Client
}

func NewTBankClient(terminalKey, secretKey, baseURL, notificationURL string) *TBankClient {
	// Логируем конфигурацию при создании (секрет маскируем)
	maskedKey := ""
	if len(secretKey) > 4 {
		maskedKey = secretKey[:4] + "****"
	}
	log.Printf("TBank client: terminal=%s secret=%s url=%s notify=%s",
		terminalKey, maskedKey, baseURL, notificationURL)

	return &TBankClient{
		terminalKey:     terminalKey,
		secretKey:       secretKey,
		baseURL:         strings.TrimRight(baseURL, "/"),
		notificationURL: notificationURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ================== Структуры ==================

type InitRequest struct {
	TerminalKey     string            `json:"TerminalKey"`
	Amount          int64             `json:"Amount"`
	OrderId         string            `json:"OrderId"`
	Description     string            `json:"Description"`
	Token           string            `json:"Token"`
	NotificationURL string            `json:"NotificationURL,omitempty"`
	DATA            map[string]string `json:"DATA,omitempty"`
}

type InitResponse struct {
	Success     bool   `json:"Success"`
	ErrorCode   string `json:"ErrorCode"`
	Message     string `json:"Message"`
	Details     string `json:"Details"`
	TerminalKey string `json:"TerminalKey"`
	Status      string `json:"Status"`
	PaymentId   int64  `json:"PaymentId"`
	OrderId     string `json:"OrderId"`
	Amount      int64  `json:"Amount"`
	PaymentURL  string `json:"PaymentURL"`
}

type NotificationRequest struct {
	TerminalKey string `json:"TerminalKey"`
	OrderId     string `json:"OrderId"`
	Success     bool   `json:"Success"`
	Status      string `json:"Status"`
	PaymentId   int64  `json:"PaymentId"`
	ErrorCode   string `json:"ErrorCode"`
	Amount      int64  `json:"Amount"`
	CardId      int64  `json:"CardId,omitempty"`
	Pan         string `json:"Pan,omitempty"`
	ExpDate     string `json:"ExpDate,omitempty"`
	Token       string `json:"Token"`
}

// ================== Init ==================

func (c *TBankClient) InitPayment(
	ctx context.Context,
	amount int64,
	description string,
	userData map[string]string,
) (*InitResponse, error) {

	// Валидация
	if c.terminalKey == "" {
		return nil, fmt.Errorf("TBANK_TERMINAL_KEY is empty")
	}
	if c.secretKey == "" {
		return nil, fmt.Errorf("TBANK_SECRET_KEY is empty")
	}
	if amount <= 0 {
		return nil, fmt.Errorf("amount must be > 0, got %d", amount)
	}

	orderID := fmt.Sprintf("sub_%d", time.Now().UnixNano())

	// Параметры для токена:
	// ВСЕ отправляемые плоские поля + Password, КРОМЕ Token и DATA
	tokenParams := map[string]string{
		"TerminalKey": c.terminalKey,
		"Amount":      fmt.Sprintf("%d", amount),
		"OrderId":     orderID,
		"Description": description,
		"Password":    c.secretKey,
	}
	if c.notificationURL != "" {
		tokenParams["NotificationURL"] = c.notificationURL
	}

	token := generateToken(tokenParams)

	req := InitRequest{
		TerminalKey:     c.terminalKey,
		Amount:          amount,
		OrderId:         orderID,
		Description:     description,
		NotificationURL: c.notificationURL,
		DATA:            userData,
		Token:           token,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	url := c.baseURL + "/Init"
	log.Printf("TBank Init → %s body=%s", url, string(body))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	log.Printf("TBank Init ← status=%d body=%s", resp.StatusCode, string(respBody))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status %d: %s", resp.StatusCode, string(respBody))
	}

	var initResp InitResponse
	if err := json.Unmarshal(respBody, &initResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if !initResp.Success {
		return nil, fmt.Errorf("tbank error [%s]: %s — %s",
			initResp.ErrorCode, initResp.Message, initResp.Details)
	}

	log.Printf("TBank payment created: OrderId=%s PaymentURL=%s",
		initResp.OrderId, initResp.PaymentURL)

	return &initResp, nil
}

// ================== Верификация нотификации ==================

func (c *TBankClient) VerifyNotification(n *NotificationRequest) bool {
	params := map[string]string{
		"TerminalKey": n.TerminalKey,
		"OrderId":     n.OrderId,
		"Success":     boolToString(n.Success),
		"Status":      n.Status,
		"PaymentId":   fmt.Sprintf("%d", n.PaymentId),
		"ErrorCode":   n.ErrorCode,
		"Amount":      fmt.Sprintf("%d", n.Amount),
		"Password":    c.secretKey,
	}
	if n.Pan != "" {
		params["Pan"] = n.Pan
	}
	if n.ExpDate != "" {
		params["ExpDate"] = n.ExpDate
	}
	if n.CardId != 0 {
		params["CardId"] = fmt.Sprintf("%d", n.CardId)
	}

	expected := generateToken(params)
	ok := expected == n.Token
	if !ok {
		log.Printf("TBank token mismatch: expected=%s got=%s", expected, n.Token)
	}
	return ok
}

// ================== Генерация токена ==================
//
// Алгоритм T-Bank:
//  1. Все пары ключ-значение + Password
//  2. Сортировка по ключу (ascending)
//  3. Конкатенация ТОЛЬКО значений (без разделителей)
//  4. SHA-256 → hex lowercase

func generateToken(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(params[k])
	}

	hash := sha256.Sum256([]byte(sb.String()))
	return fmt.Sprintf("%x", hash)
}

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
