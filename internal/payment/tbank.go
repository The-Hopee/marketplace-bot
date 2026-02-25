package payment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type TBankClient struct {
	terminalKey string
	secretKey   string
	baseURL     string
	client      *http.Client
}

type InitRequest struct {
	TerminalKey string            `json:"TerminalKey"`
	Amount      int64             `json:"Amount"`
	OrderId     string            `json:"OrderId"`
	Description string            `json:"Description"`
	Token       string            `json:"Token"`
	DATA        map[string]string `json:"DATA,omitempty"`
	Receipt     *Receipt          `json:"Receipt,omitempty"`
}

type Receipt struct {
	Email    string        `json:"Email,omitempty"`
	Phone    string        `json:"Phone,omitempty"`
	Taxation string        `json:"Taxation"`
	Items    []ReceiptItem `json:"Items"`
}

type ReceiptItem struct {
	Name     string  `json:"Name"`
	Price    int64   `json:"Price"`
	Quantity float64 `json:"Quantity"`
	Amount   int64   `json:"Amount"`
	Tax      string  `json:"Tax"`
}

type InitResponse struct {
	Success     bool   `json:"Success"`
	ErrorCode   string `json:"ErrorCode"`
	TerminalKey string `json:"TerminalKey"`
	Status      string `json:"Status"`
	PaymentId   string `json:"PaymentId"`
	OrderId     string `json:"OrderId"`
	Amount      int64  `json:"Amount"`
	PaymentURL  string `json:"PaymentURL"`
	Message     string `json:"Message"`
	Details     string `json:"Details"`
}

type NotificationRequest struct {
	TerminalKey string `json:"TerminalKey"`
	OrderId     string `json:"OrderId"`
	Success     bool   `json:"Success"`
	Status      string `json:"Status"`
	PaymentId   int64  `json:"PaymentId"`
	ErrorCode   string `json:"ErrorCode"`
	Amount      int64  `json:"Amount"`
	Pan         string `json:"Pan"`
	Token       string `json:"Token"`
}

type GetStateRequest struct {
	TerminalKey string `json:"TerminalKey"`
	PaymentId   string `json:"PaymentId"`
	Token       string `json:"Token"`
}

type GetStateResponse struct {
	Success   bool   `json:"Success"`
	ErrorCode string `json:"ErrorCode"`
	Status    string `json:"Status"`
	PaymentId string `json:"PaymentId"`
	OrderId   string `json:"OrderId"`
	Amount    int64  `json:"Amount"`
}

func NewTBankClient(terminalKey, secretKey, baseURL string) *TBankClient {
	return &TBankClient{
		terminalKey: terminalKey,
		secretKey:   secretKey,
		baseURL:     baseURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *TBankClient) generateToken(params map[string]string) string {
	// Добавляем Password (секретный ключ)
	params["Password"] = c.secretKey

	// Сортируем ключи
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Конкатенируем значения
	var values strings.Builder
	for _, k := range keys {
		values.WriteString(params[k])
	}

	// SHA256
	hash := sha256.Sum256([]byte(values.String()))
	return fmt.Sprintf("%x", hash)
}

func (c *TBankClient) InitPayment(ctx context.Context, amount int64, description string, userData map[string]string) (*InitResponse, error) {
	orderID := uuid.New().String()

	params := map[string]string{
		"TerminalKey": c.terminalKey,
		"Amount":      fmt.Sprintf("%d", amount),
		"OrderId":     orderID,
		"Description": description,
	}

	token := c.generateToken(params)

	reqData := InitRequest{
		TerminalKey: c.terminalKey,
		Amount:      amount,
		OrderId:     orderID,
		Description: description,
		Token:       token,
		DATA:        userData,
		Receipt: &Receipt{
			Taxation: "usn_income", // УСН доходы
			Items: []ReceiptItem{
				{
					Name:     "Подписка на поиск по маркетплейсам (1 месяц)",
					Price:    amount,
					Quantity: 1,
					Amount:   amount,
					Tax:      "none",
				},
			},
		},
	}

	jsonData, err := json.Marshal(reqData)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/Init", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var initResp InitResponse
	if err := json.Unmarshal(body, &initResp); err != nil {
		return nil, err
	}

	if !initResp.Success {
		return nil, fmt.Errorf("payment init failed: %s - %s", initResp.ErrorCode, initResp.Message)
	}

	return &initResp, nil
}

func (c *TBankClient) GetState(ctx context.Context, paymentID string) (*GetStateResponse, error) {
	params := map[string]string{
		"TerminalKey": c.terminalKey,
		"PaymentId":   paymentID,
	}

	token := c.generateToken(params)

	reqData := GetStateRequest{
		TerminalKey: c.terminalKey,
		PaymentId:   paymentID,
		Token:       token,
	}

	jsonData, err := json.Marshal(reqData)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/GetState", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var stateResp GetStateResponse
	if err := json.Unmarshal(body, &stateResp); err != nil {
		return nil, err
	}

	return &stateResp, nil
}

func (c *TBankClient) VerifyNotification(notification *NotificationRequest) bool {
	params := map[string]string{
		"TerminalKey": notification.TerminalKey,
		"OrderId":     notification.OrderId,
		"Success":     fmt.Sprintf("%t", notification.Success),
		"Status":      notification.Status,
		"PaymentId":   fmt.Sprintf("%d", notification.PaymentId),
		"ErrorCode":   notification.ErrorCode,
		"Amount":      fmt.Sprintf("%d", notification.Amount),
	}

	expectedToken := c.generateToken(params)
	return notification.Token == expectedToken
}
