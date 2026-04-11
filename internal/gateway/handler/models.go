package handler

import (
	"net/http"

	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/gin-gonic/gin"
)

type ModelsHandler struct {
	registry *provider.Registry
}

func NewModelsHandler(registry *provider.Registry) *ModelsHandler {
	return &ModelsHandler{registry: registry}
}

func (h *ModelsHandler) List(c *gin.Context) {
	includeAliases := c.Query("include_aliases") == "true"
	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   h.registry.ListModels(includeAliases),
	})
}
