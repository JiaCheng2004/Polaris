package handler

import (
	"encoding/json"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/gin-gonic/gin"
)

func writeSSEData(c *gin.Context, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return writeRawSSEData(c, payload)
}

func writeRawSSEData(c *gin.Context, payload []byte) error {
	if _, err := c.Writer.Write([]byte("data: ")); err != nil {
		return err
	}
	if _, err := c.Writer.Write(payload); err != nil {
		return err
	}
	if _, err := c.Writer.Write([]byte("\n\n")); err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}

func writeRawSSEEvent(c *gin.Context, event string, payload []byte) error {
	if strings.TrimSpace(event) != "" {
		if _, err := c.Writer.Write([]byte("event: ")); err != nil {
			return err
		}
		if _, err := c.Writer.Write([]byte(strings.TrimSpace(event))); err != nil {
			return err
		}
		if _, err := c.Writer.Write([]byte("\n")); err != nil {
			return err
		}
	}
	return writeRawSSEData(c, payload)
}

func writeSSEDone(c *gin.Context) error {
	if _, err := c.Writer.Write([]byte("data: [DONE]\n\n")); err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}

func writeSSEErrorEvent(c *gin.Context, event string, apiErr *httputil.APIError) error {
	if apiErr == nil {
		apiErr = httputil.NewError(500, "internal_error", "internal_error", "", "An internal error occurred.")
	}
	payload, err := json.Marshal(httputil.ErrorEnvelope{
		Error: httputil.ErrorBody{
			Message: apiErr.Message,
			Type:    apiErr.Type,
			Code:    apiErr.Code,
			Param:   apiErr.Param,
		},
	})
	if err != nil {
		return err
	}
	return writeRawSSEEvent(c, event, payload)
}
