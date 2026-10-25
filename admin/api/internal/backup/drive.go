package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Drive's endpoints. Variables so a test can point them at an httptest server.
var (
	driveFilesURL  = "https://www.googleapis.com/drive/v3/files"
	driveUploadURL = "https://www.googleapis.com/upload/drive/v3/files"
)

// The appProperties every upload carries. They are what retention selects on, so
// a file the panel did not upload — even one sitting in the same folder with a
// name that looks like a backup — is never a candidate for the trash.
const (
	propertySchedule = "homeLabSchedule"
	propertyDatabase = "homeLabDatabase"
)

// driveFields is what is asked of Drive for every file, and all File needs.
const driveFields = "id,name,size,createdTime,appProperties,webViewLink"

// drivePageSize is the most Drive returns per page.
const drivePageSize = "1000"

// DriveClient makes the handful of Drive calls backups need.
//
// Hand-rolled over google.golang.org/api on purpose. That client is generated for
// all of Drive and would bring a large dependency for four requests; these are
// plain REST, and the oauth2 HTTP client underneath already handles the token.
type DriveClient struct {
	// httpClient carries the grant: it is oauth2's, and it adds and refreshes the
	// bearer token on every request.
	httpClient *http.Client
}

// driveFile is the wire shape of a Drive file.
type driveFile struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Size          string            `json:"size"`
	CreatedTime   time.Time         `json:"createdTime"`
	AppProperties map[string]string `json:"appProperties"`
	WebViewLink   string            `json:"webViewLink"`
}

func (f driveFile) backupFile() File {
	size, _ := strconv.ParseInt(f.Size, 10, 64)
	return File{
		ID:          f.ID,
		Name:        f.Name,
		Database:    f.AppProperties[propertyDatabase],
		Size:        size,
		CreatedAt:   f.CreatedTime,
		WebViewLink: f.WebViewLink,
	}
}

// Upload stores one backup in a folder, tagged with the schedule and database.
//
// A resumable upload, in its simplest form: one request to open the session,
// one to send the whole body. It is the form Drive recommends above 5MB, and it
// means the body is streamed from disk rather than held in memory.
func (c *DriveClient) Upload(ctx context.Context, folderID, name, schedule, database string,
	body io.Reader, size int64,
) (File, error) {
	metadata, err := json.Marshal(map[string]any{
		"name":          name,
		"parents":       []string{folderID},
		"appProperties": map[string]string{propertySchedule: schedule, propertyDatabase: database},
	})
	if err != nil {
		return File{}, fmt.Errorf("encode the upload metadata: %w", err)
	}

	open, err := http.NewRequestWithContext(ctx, http.MethodPost,
		driveUploadURL+"?uploadType=resumable&fields="+driveFields, bytes.NewReader(metadata))
	if err != nil {
		return File{}, fmt.Errorf("build the upload request: %w", err)
	}
	open.Header.Set("Content-Type", "application/json; charset=UTF-8")
	open.Header.Set("X-Upload-Content-Type", "application/octet-stream")
	open.Header.Set("X-Upload-Content-Length", strconv.FormatInt(size, 10))

	var ignored struct{}
	session, err := c.do(open, "start uploading "+name, &ignored)
	if err != nil {
		return File{}, err
	}
	location := session.Get("Location")
	if location == "" {
		return File{}, fmt.Errorf("%w: Drive opened no upload session for %s", ErrGoogleRefused, name)
	}

	send, err := http.NewRequestWithContext(ctx, http.MethodPut, location, body)
	if err != nil {
		return File{}, fmt.Errorf("build the upload body: %w", err)
	}
	send.ContentLength = size
	send.Header.Set("Content-Type", "application/octet-stream")

	var uploaded driveFile
	if _, err := c.do(send, "upload "+name, &uploaded); err != nil {
		return File{}, err
	}
	return uploaded.backupFile(), nil
}

// List returns every backup a schedule has in a folder, trashed ones excluded.
//
// The query is assembled from validated values only — a folder id and a schedule
// name, both allowlisted before they get here — which is why nothing is escaped.
func (c *DriveClient) List(ctx context.Context, folderID, schedule string) ([]File, error) {
	query := fmt.Sprintf("'%s' in parents and trashed = false and "+
		"appProperties has { key='%s' and value='%s' }", folderID, propertySchedule, schedule)

	var ret []File
	pageToken := ""
	for {
		values := url.Values{
			"q":        {query},
			"fields":   {"nextPageToken,files(" + driveFields + ")"},
			"pageSize": {drivePageSize},
		}
		if pageToken != "" {
			values.Set("pageToken", pageToken)
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, driveFilesURL+"?"+values.Encode(), nil)
		if err != nil {
			return nil, fmt.Errorf("build the listing request: %w", err)
		}

		var page struct {
			NextPageToken string      `json:"nextPageToken"`
			Files         []driveFile `json:"files"`
		}
		if _, err := c.do(request, "list the backups in the folder", &page); err != nil {
			return nil, err
		}
		for _, file := range page.Files {
			ret = append(ret, file.backupFile())
		}
		if page.NextPageToken == "" {
			return ret, nil
		}
		pageToken = page.NextPageToken
	}
}

// Trash moves a backup to Drive's trash, which empties itself after 30 days. Not
// a delete: a schedule set to keep too few can be undone for a month.
func (c *DriveClient) Trash(ctx context.Context, fileID string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPatch,
		driveFilesURL+"/"+url.PathEscape(fileID)+"?fields=id", strings.NewReader(`{"trashed":true}`))
	if err != nil {
		return fmt.Errorf("build the trash request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json; charset=UTF-8")

	var ignored struct{}
	_, err = c.do(request, "move "+fileID+" to the trash", &ignored)
	return err
}

// do sends one request and decodes the answer, sorting failures the way the rest
// of the slice does: Google answering no is a refusal with Google's own sentence,
// and Google not answering is unreachable.
func (c *DriveClient) do(request *http.Request, what string, into any) (http.Header, error) {
	response, err := c.httpClient.Do(request)
	if err != nil {
		// The oauth2 transport reports a dead grant as a transport error, so this
		// is also where "the refresh token was revoked" arrives.
		if strings.Contains(err.Error(), "oauth2:") {
			return nil, fmt.Errorf("%w: %s: %w", ErrGoogleRefused, what, err)
		}
		return nil, fmt.Errorf("%w: %s: %w", ErrGoogleUnreachable, what, err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode >= http.StatusBadRequest {
		var failure struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.NewDecoder(response.Body).Decode(&failure)
		return nil, fmt.Errorf("%w: %s: Drive answered %d: %s",
			ErrGoogleRefused, what, response.StatusCode, failure.Error.Message)
	}

	if err := json.NewDecoder(response.Body).Decode(into); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: read Drive's answer: %w", what, err)
	}
	return response.Header, nil
}
