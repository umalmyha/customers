package handlers

import (
	"fmt"
	"github.com/labstack/echo/v4"
	"io"
	"net/http"
	"os"
)

const mimeBytesNumber = 512

type ImageHandler struct {
	validImgMimeTypes map[string]struct{}
}

func NewImageHandler() *ImageHandler {
	return &ImageHandler{
		validImgMimeTypes: map[string]struct{}{
			"image/gif":                {},
			"image/jpeg":               {},
			"image/pjpeg":              {},
			"image/png":                {},
			"image/svg+xml":            {},
			"image/tiff":               {},
			"image/vnd.microsoft.icon": {},
			"image/vnd.wap.wbmp":       {},
			"image/webp":               {},
		},
	}
}

func (h *ImageHandler) Upload(c echo.Context) error {
	fileHdr, err := c.FormFile("image")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	file, err := fileHdr.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("failed to load file content - %v", err))
	}
	defer file.Close()

	mimeBuff := make([]byte, mimeBytesNumber)
	_, err = file.Read(mimeBuff)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	mimeType := http.DetectContentType(mimeBuff)
	if !h.isMimeTypeAllowed(mimeType) {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("MIME type %s is not allowed", mimeType))
	}

	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	path := fmt.Sprintf("./images/%s", fileHdr.Filename)
	dst, err := os.Create(path)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.NoContent(http.StatusOK)
}

func (h *ImageHandler) Download(c echo.Context) error {
	name := c.Param("name")
	path := fmt.Sprintf("./images/%s", name)
	return c.Attachment(path, name)
}

func (h *ImageHandler) isMimeTypeAllowed(mime string) bool {
	if _, ok := h.validImgMimeTypes[mime]; ok {
		return true
	}
	return false
}
