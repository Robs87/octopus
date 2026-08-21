package relay

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/looplj/axonhub/llm"
)

// writeRelayError keeps relay failures compatible with the protocol used by
// the client. The generic admin response shape (code/message at the root) is
// not understood by OpenAI-compatible clients and gets reported as
// "status code (no body)" even when a JSON body was sent.
func writeRelayError(c *gin.Context, inboundType llm.APIFormat, statusCode int, err error) {
	message := "request failed"
	if err != nil {
		message = err.Error()
	}

	if inboundType == llm.APIFormatAnthropicMessage {
		c.AbortWithStatusJSON(statusCode, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "api_error",
				"message": message,
			},
		})
		return
	}

	c.AbortWithStatusJSON(statusCode, gin.H{
		"error": gin.H{
			"message": message,
			"type":    "server_error",
			// OpenAI-compatible clients commonly decode code as a string.
			"code": strconv.Itoa(statusCode),
		},
	})
}
