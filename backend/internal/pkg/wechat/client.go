package wechat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"time"
)

const BaseURL = "https://api.weixin.qq.com/cgi-bin"

type Client struct {
	AppID     string
	AppSecret string
	token     string
	expiry    time.Time
}

type ErrorResponse struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	ErrorResponse
}

type MediaResponse struct {
	MediaID string `json:"media_id"`
	URL     string `json:"url"`
	ErrorResponse
}

type DraftResponse struct {
	MediaID string `json:"media_id"`
	ErrorResponse
}

type PublishResponse struct {
	PublishID string `json:"publish_id"`
	ErrorResponse
}

func NewClient(appID, appSecret string) *Client {
	return &Client{
		AppID:     appID,
		AppSecret: appSecret,
	}
}

func (c *Client) GetToken() (string, error) {
	if c.token != "" && time.Now().Before(c.expiry) {
		return c.token, nil
	}

	u := fmt.Sprintf("%s/token?grant_type=client_credential&appid=%s&secret=%s", BaseURL, c.AppID, c.AppSecret)
	resp, err := http.Get(u)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var tr TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", err
	}

	if tr.ErrCode != 0 {
		return "", fmt.Errorf("wechat error %d: %s", tr.ErrCode, tr.ErrMsg)
	}

	c.token = tr.AccessToken
	c.expiry = time.Now().Add(time.Duration(tr.ExpiresIn-60) * time.Second)
	return c.token, nil
}

func (c *Client) UploadImage(imageBytes []byte, filename string) (*MediaResponse, error) {
	token, err := c.GetToken()
	if err != nil {
		return nil, err
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("media", filename)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(imageBytes); err != nil {
		return nil, err
	}
	writer.Close()

	u := fmt.Sprintf("%s/material/add_material?access_token=%s&type=image", BaseURL, token)
	req, err := http.NewRequest("POST", u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var mr MediaResponse
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return nil, err
	}

	if mr.ErrCode != 0 {
		return nil, fmt.Errorf("wechat error %d: %s", mr.ErrCode, mr.ErrMsg)
	}

	return &mr, nil
}

type Article struct {
	Title            string `json:"title"`
	ThumbMediaID     string `json:"thumb_media_id"`
	Author           string `json:"author"`
	Digest           string `json:"digest"`
	Content          string `json:"content"`
	ContentSourceURL string `json:"content_source_url"`
	NeedOpenComment  int    `json:"need_open_comment"`
	OnlyFansCanComment int  `json:"only_fans_can_comment"`
}

func (c *Client) CreateDraft(articles []Article) (string, error) {
	token, err := c.GetToken()
	if err != nil {
		return "", err
	}

	payload := map[string]interface{}{
		"articles": articles,
	}
	body, _ := json.Marshal(payload)

	u := fmt.Sprintf("%s/draft/add?access_token=%s", BaseURL, token)
	resp, err := http.Post(u, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var dr DraftResponse
	if err := json.NewDecoder(resp.Body).Decode(&dr); err != nil {
		return "", err
	}

	if dr.ErrCode != 0 {
		return "", fmt.Errorf("wechat error %d: %s", dr.ErrCode, dr.ErrMsg)
	}

	return dr.MediaID, nil
}

func (c *Client) Publish(mediaID string) (string, int, error) {
	token, err := c.GetToken()
	if err != nil {
		return "", 0, err
	}

	payload := map[string]string{"media_id": mediaID}
	body, _ := json.Marshal(payload)

	u := fmt.Sprintf("%s/freepublish/submit?access_token=%s", BaseURL, token)
	resp, err := http.Post(u, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	var pr PublishResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return "", 0, err
	}

	return pr.PublishID, pr.ErrCode, nil
}
