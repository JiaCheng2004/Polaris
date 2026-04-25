package handler

import (
	"net/http"

	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/gin-gonic/gin"
)

type ModelsHandler struct {
	runtime *gwruntime.Holder
}

func NewModelsHandler(runtime *gwruntime.Holder) *ModelsHandler {
	return &ModelsHandler{runtime: runtime}
}

func (h *ModelsHandler) List(c *gin.Context) {
	includeAliases := c.Query("include_aliases") == "true"
	registry := h.registry(c)
	if registry == nil {
		c.JSON(http.StatusOK, gin.H{"object": "list", "data": []provider.Model{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   registry.ListModels(includeAliases),
	})
}

func (h *ModelsHandler) registry(c *gin.Context) *provider.Registry {
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if snapshot == nil {
		return nil
	}
	return snapshot.Registry
}
