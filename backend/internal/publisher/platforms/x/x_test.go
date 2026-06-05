package x

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	"gorm.io/datatypes"

	"github.com/kurodakayn/mpp-backend/internal/models"
	pkgx "github.com/kurodakayn/mpp-backend/internal/pkg/x"
)

type fakeXTweetClient struct {
	text string
	err  error
}

func (f *fakeXTweetClient) CreateTweet(ctx context.Context, text string) (pkgx.Tweet, error) {
	f.text = text
	if f.err != nil {
		return pkgx.Tweet{}, f.err
	}
	return pkgx.Tweet{ID: "tweet-1", Text: text}, nil
}

func TestXWeightedLengthCountsCJKAndEmojiAsDouble(t *testing.T) {
	text := "abc\u4e2d\u6587\U0001F600"

	if got := xWeightedLength(text); got != 9 {
		t.Fatalf("expected weighted length 9, got %d", got)
	}
}

func TestXWeightedLengthCountsURLsAsTransformedLength(t *testing.T) {
	text := "go https://example.com/really/long/path"

	if got := xWeightedLength(text); got != 26 {
		t.Fatalf("expected URL weighted length 26, got %d", got)
	}
}

func TestBuildXPostIntentURLUsesAdaptedText(t *testing.T) {
	intentURL, err := BuildXPostIntentURL(datatypes.JSON(`{"text":"hello x & \u4e2d\u6587"}`))
	if err != nil {
		t.Fatalf("expected intent URL, got %v", err)
	}

	parsed, err := url.Parse(intentURL)
	if err != nil {
		t.Fatalf("expected valid URL, got %v", err)
	}
	if parsed.Scheme != "https" || parsed.Host != "x.com" || parsed.Path != "/intent/tweet" {
		t.Fatalf("unexpected intent URL: %s", intentURL)
	}
	if got := parsed.Query().Get("text"); got != "hello x & \u4e2d\u6587" {
		t.Fatalf("expected text query to round-trip, got %q", got)
	}
}

func TestXPublisherPublishUsesOAuth2AccountCredentials(t *testing.T) {
	originalOAuth1Client := newXOAuth1TweetClient
	originalOAuth2Client := newXOAuth2TweetClient
	defer func() {
		newXOAuth1TweetClient = originalOAuth1Client
		newXOAuth2TweetClient = originalOAuth2Client
	}()

	oauth1Called := false
	oauth2Client := &fakeXTweetClient{}
	newXOAuth1TweetClient = func(creds pkgx.Credentials) xTweetClient {
		oauth1Called = true
		return &fakeXTweetClient{err: fmt.Errorf("unexpected oauth1 publish")}
	}
	newXOAuth2TweetClient = func(creds pkgx.OAuth2Credentials) xTweetClient {
		if creds.AccessToken != "oauth2-access" {
			t.Fatalf("expected oauth2 access token, got %q", creds.AccessToken)
		}
		return oauth2Client
	}

	pub := &models.ProjectPlatformPublication{
		Config:         datatypes.JSON(`{"api_key":"stale","api_secret":"stale","access_token":"stale","access_token_secret":"stale"}`),
		AdaptedContent: datatypes.JSON(`{"text":"hello from oauth2"}`),
	}
	account := &models.PlatformAccount{
		Credentials: datatypes.JSON(`{"auth_type":"oauth2","oauth2_access_token":"oauth2-access","username":"creator"}`),
	}

	remoteID, publishURL, err := (&XPublisher{}).Publish(context.Background(), pub, account)
	if err != nil {
		t.Fatalf("expected oauth2 publish to succeed, got %v", err)
	}
	if oauth1Called {
		t.Fatalf("expected oauth1 publisher not to be used")
	}
	if remoteID != "tweet-1" {
		t.Fatalf("expected remote id tweet-1, got %q", remoteID)
	}
	if publishURL != "https://x.com/creator/status/tweet-1" {
		t.Fatalf("expected username status URL, got %q", publishURL)
	}
	if oauth2Client.text != "hello from oauth2" {
		t.Fatalf("expected oauth2 tweet text, got %q", oauth2Client.text)
	}
}
