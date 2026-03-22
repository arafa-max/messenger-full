package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	apiKey  string
	from    string
	httpCli *http.Client
}

func NewResendClient(apiKey, from string) *Client {
	return &Client{
		apiKey: apiKey,
		from:   from,
		httpCli: &http.Client{Timeout: 10 * time.Second},
	}
}

type sendEmailRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
}

func (c *Client) SendMagicLink(ctx context.Context, toEmail, magicURL string) error {
	body := sendEmailRequest{
		From:    c.from,
		To:      []string{toEmail},
		Subject: "Ваша ссылка для входа",
		HTML:    buildMagicLinkHTML(magicURL),
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.resend.com/emails", bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpCli.Do(req)
	if err != nil {
		return fmt.Errorf("send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errBody map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errBody)
		return fmt.Errorf("resend error %d: %v", resp.StatusCode, errBody)
	}
	return nil
}

func buildMagicLinkHTML(url string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<body style="font-family:sans-serif;max-width:480px;margin:40px auto;padding:0 20px">
  <h2 style="font-size:20px;font-weight:500;margin-bottom:8px">Вход в мессенджер</h2>
  <p style="color:#666;margin-bottom:24px">Нажмите кнопку ниже чтобы войти. Ссылка действует 15 минут.</p>
  <a href="%s"
     style="display:inline-block;padding:12px 24px;background:#000;color:#fff;
            text-decoration:none;border-radius:8px;font-size:15px">
    Войти
  </a>
  <p style="color:#999;font-size:12px;margin-top:24px">
    Если вы не запрашивали вход — просто проигнорируйте это письмо.
  </p>
</body>
</html>`, url)
}