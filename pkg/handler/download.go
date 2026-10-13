package handler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gocarina/gocsv"
	"github.com/mark3labs/mcp-go/mcp"
	"go.uber.org/zap"
)

const maxDownloadFiles = 50

var (
	fileIDPattern      = regexp.MustCompile(`\bF[A-Z0-9]{6,}\b`)
	unsafeNameChars    = regexp.MustCompile(`[^A-Za-z0-9._ -]+`)
	repeatedUnderscore = regexp.MustCompile(`_+`)
)

type downloadedFile struct {
	FileID   string `csv:"FileID"`
	Filename string `csv:"Filename"`
	Mimetype string `csv:"Mimetype"`
	Size     int64  `csv:"Size"`
	Error    string `csv:"Error"`
}

// AttachmentDownloadHandler saves one or more Slack files into SLACK_MCP_DOWNLOAD_DIR
// and returns only their metadata, so file contents never enter the model context.
func (ch *ConversationsHandler) AttachmentDownloadHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ch.logger.Debug("AttachmentDownloadHandler called", zap.Any("params", request.Params))

	dir := os.Getenv("SLACK_MCP_DOWNLOAD_DIR")
	if dir == "" {
		return nil, errors.New("attachment_download is disabled: set SLACK_MCP_DOWNLOAD_DIR to a writable directory")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("download dir %s is not writable: %v", dir, err)
	}

	fileIDs := parseFileIDs(request.GetString("file_ids", ""))
	if len(fileIDs) == 0 {
		return nil, errors.New("file_ids is required: one or more file IDs like F0123ABCDEF, comma separated")
	}
	if len(fileIDs) > maxDownloadFiles {
		return nil, fmt.Errorf("too many files: %d requested, maximum is %d per call", len(fileIDs), maxDownloadFiles)
	}

	if ready, err := ch.apiProvider.IsReady(); !ready {
		ch.logger.Error("API provider not ready", zap.Error(err))
		return nil, err
	}

	results := make([]downloadedFile, 0, len(fileIDs))
	for _, id := range fileIDs {
		res, err := ch.downloadFile(ctx, dir, id)
		if err != nil {
			ch.logger.Error("attachment download failed", zap.String("file_id", id), zap.Error(err))
			res.Error = err.Error()
		}
		results = append(results, res)
	}

	csvBytes, err := gocsv.MarshalBytes(&results)
	if err != nil {
		return nil, err
	}
	return mcp.NewToolResultText(string(csvBytes)), nil
}

func (ch *ConversationsHandler) downloadFile(ctx context.Context, dir, fileID string) (downloadedFile, error) {
	res := downloadedFile{FileID: fileID}

	info, _, _, err := ch.apiProvider.Slack().GetFileInfoContext(ctx, fileID, 0, 0)
	if err != nil {
		return res, err
	}
	res.Mimetype = info.Mimetype

	downloadURL := info.URLPrivateDownload
	if downloadURL == "" {
		downloadURL = info.URLPrivate
	}
	if downloadURL == "" {
		return res, errors.New("file has no downloadable URL")
	}

	name := downloadFilename(info.Name, info.Title, fileID)
	tmp, err := os.CreateTemp(dir, ".partial-*")
	if err != nil {
		return res, err
	}
	defer os.Remove(tmp.Name())

	if err := ch.apiProvider.Slack().GetFileContext(ctx, downloadURL, tmp); err != nil {
		tmp.Close()
		return res, err
	}
	stat, err := tmp.Stat()
	if err != nil {
		tmp.Close()
		return res, err
	}
	if err := tmp.Close(); err != nil {
		return res, err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return res, err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, name)); err != nil {
		return res, err
	}

	res.Filename = name
	res.Size = stat.Size()
	return res, nil
}

// parseFileIDs accepts IDs in any separator style, including the
// "F123 (a.pdf), F456 (b.pdf)" form of the AttachmentIDs column.
func parseFileIDs(raw string) []string {
	seen := map[string]bool{}
	var ids []string
	for _, id := range fileIDPattern.FindAllString(raw, -1) {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

// downloadFilename builds "<stem>_<id8><ext>" so repeated names never collide.
func downloadFilename(name, title, fileID string) string {
	if name == "" {
		name = title
	}
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)

	clean := func(s string) string {
		s = unsafeNameChars.ReplaceAllString(s, "_")
		s = repeatedUnderscore.ReplaceAllString(s, "_")
		return strings.Trim(s, "._ -")
	}
	stem = clean(stem)
	ext = clean(ext)
	if stem == "" {
		stem = "slack_file"
	}
	if len(stem) > 120 {
		stem = stem[:120]
	}
	if ext != "" {
		ext = "." + ext
	}

	id := strings.ToLower(fileID)
	if len(id) > 8 {
		id = id[len(id)-8:]
	}
	return fmt.Sprintf("%s_%s%s", stem, id, ext)
}
