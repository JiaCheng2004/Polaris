package middleware

import (
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/gin-gonic/gin"
)

func Runtime(holder *gwruntime.Holder) gin.HandlerFunc {
	return func(c *gin.Context) {
		if holder != nil {
			SetRuntimeSnapshot(c, holder.Current())
		}
		c.Next()
	}
}
