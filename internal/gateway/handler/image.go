package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	cachepkg "github.com/JiaCheng2004/Polaris/internal/store/cache"
	"github.com/gin-gonic/gin"
)

type ImageHandler struct {
	runtime *gwruntime.Holder
	cache   cachepkg.Cache
}

func NewImageHandler(runtime *gwruntime.Holder, cache cachepkg.Cache) *ImageHandler {
	return &ImageHandler{runtime: runtime, cache: cache}
}

func (h *ImageHandler) Generate(c *gin.Context) {
	var req modality.ImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}
	if err := validateImageGenerationRequest(&req); err != nil {
		httputil.WriteError(c, err)
		return
	}

	registry := h.registry(c)
	if registry == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "model", "Model registry is unavailable."))
		return
	}

	required := []modality.Capability{modality.CapabilityGeneration}
	if len(req.ReferenceImages) > 0 {
		required = append(required, modality.CapabilityMultiReference)
	}

	auth := middleware.GetAuthContext(c)
	resolved, err := resolveEndpointModel(c.Request.Context(), registry, auth, req.Model, req.Routing, modality.ModalityImage, required...)
	if err != nil {
		writeModalityTargetError(c, err, "images")
		return
	}
	applyResolvedRoutingHeaders(c, resolved)
	model := resolved.Model

	adapter, _, err := registry.GetImageAdapter(model.ID)
	if err != nil {
		writeModalityTargetError(c, err, "images")
		return
	}

	cacheCtl := newResponseCache(c, h.runtime, h.cache)
	cacheKey := exactCacheKey("image-generate", model.ID, req)
	if cacheCtl != nil && cacheCtl.tryExact(c, cacheKey, model, modality.ModalityImage) {
		return
	}
	if cacheCtl == nil {
		c.Header(cacheHeader, "bypass")
	}

	req.Model = model.ID
	response, err := adapter.Generate(c.Request.Context(), &req)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	middleware.SetRequestOutcome(c, middleware.RequestOutcome{
		Model:      model.ID,
		Provider:   model.Provider,
		Modality:   modality.ModalityImage,
		StatusCode: http.StatusOK,
	})
	if cacheCtl != nil {
		cacheCtl.storeJSON(c, cacheKey, http.StatusOK, response)
	}
	c.JSON(http.StatusOK, response)
}

func (h *ImageHandler) Edit(c *gin.Context) {
	req, err := parseImageEditRequest(c)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	if err := validateImageEditRequest(req); err != nil {
		httputil.WriteError(c, err)
		return
	}

	registry := h.registry(c)
	if registry == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "model", "Model registry is unavailable."))
		return
	}

	auth := middleware.GetAuthContext(c)
	resolved, err := resolveEndpointModel(c.Request.Context(), registry, auth, req.Model, req.Routing, modality.ModalityImage, modality.CapabilityEditing)
	if err != nil {
		writeModalityTargetError(c, err, "images")
		return
	}
	applyResolvedRoutingHeaders(c, resolved)
	model := resolved.Model

	adapter, _, err := registry.GetImageAdapter(model.ID)
	if err != nil {
		writeModalityTargetError(c, err, "images")
		return
	}

	cacheCtl := newResponseCache(c, h.runtime, h.cache)
	cacheKey := exactCacheKey("image-edit", model.ID, map[string]any{
		"prompt":          req.Prompt,
		"image":           hashBytes(req.Image),
		"mask":            hashBytes(req.Mask),
		"n":               req.N,
		"size":            req.Size,
		"response_format": req.ResponseFormat,
	})
	if cacheCtl != nil && cacheCtl.tryExact(c, cacheKey, model, modality.ModalityImage) {
		return
	}
	if cacheCtl == nil {
		c.Header(cacheHeader, "bypass")
	}

	req.Model = model.ID
	response, err := adapter.Edit(c.Request.Context(), req)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	middleware.SetRequestOutcome(c, middleware.RequestOutcome{
		Model:      model.ID,
		Provider:   model.Provider,
		Modality:   modality.ModalityImage,
		StatusCode: http.StatusOK,
	})
	if cacheCtl != nil {
		cacheCtl.storeJSON(c, cacheKey, http.StatusOK, response)
	}
	c.JSON(http.StatusOK, response)
}

func (h *ImageHandler) registry(c *gin.Context) *provider.Registry {
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if snapshot == nil {
		return nil
	}
	return snapshot.Registry
}

func parseImageEditRequest(c *gin.Context) (*modality.ImageEditRequest, error) {
	imageHeader, err := c.FormFile("image")
	if err != nil {
		if httputil.IsRequestBodyTooLarge(err) {
			return nil, httputil.RequestBodyTooLargeError(0)
		}
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_image", "image", "Form field 'image' is required.")
	}
	imageData, imageType, err := readMultipartFile(imageHeader)
	if err != nil {
		if httputil.IsRequestBodyTooLarge(err) {
			return nil, httputil.RequestBodyTooLargeError(0)
		}
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_image", "image", "Unable to read uploaded image.")
	}

	req := &modality.ImageEditRequest{
		Model:          c.PostForm("model"),
		Prompt:         c.PostForm("prompt"),
		Image:          imageData,
		ImageFilename:  imageHeader.Filename,
		ImageType:      imageType,
		Size:           c.PostForm("size"),
		ResponseFormat: c.DefaultPostForm("response_format", "url"),
	}
	req.Routing, err = parseRoutingFormValue(c.PostForm("routing"))
	if err != nil {
		return nil, err
	}

	if n := strings.TrimSpace(c.PostForm("n")); n != "" {
		parsed, err := strconv.Atoi(n)
		if err != nil {
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_n", "n", "Field 'n' must be an integer.")
		}
		req.N = parsed
	}
	if req.N == 0 {
		req.N = 1
	}

	if maskHeader, err := c.FormFile("mask"); err == nil {
		maskData, maskType, readErr := readMultipartFile(maskHeader)
		if readErr != nil {
			if httputil.IsRequestBodyTooLarge(readErr) {
				return nil, httputil.RequestBodyTooLargeError(0)
			}
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_mask", "mask", "Unable to read uploaded mask.")
		}
		req.Mask = maskData
		req.MaskFilename = maskHeader.Filename
		req.MaskType = maskType
	} else if httputil.IsRequestBodyTooLarge(err) {
		return nil, httputil.RequestBodyTooLargeError(0)
	}

	return req, nil
}

func validateImageGenerationRequest(req *modality.ImageRequest) error {
	if strings.TrimSpace(req.Model) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_model", "model", "Field 'model' is required.")
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_prompt", "prompt", "Field 'prompt' is required.")
	}
	if req.N == 0 {
		req.N = 1
	}
	if req.N < 1 || req.N > 10 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_n", "n", "Field 'n' must be between 1 and 10.")
	}
	switch req.ResponseFormat {
	case "", "url", "b64_json":
	default:
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_response_format", "response_format", "Field 'response_format' must be 'url' or 'b64_json'.")
	}
	if req.ResponseFormat == "" {
		req.ResponseFormat = "url"
	}
	for _, image := range req.ReferenceImages {
		if strings.TrimSpace(image) == "" {
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_reference_images", "reference_images", "Field 'reference_images' must not contain empty entries.")
		}
	}
	return nil
}

func validateImageEditRequest(req *modality.ImageEditRequest) error {
	if strings.TrimSpace(req.Model) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_model", "model", "Field 'model' is required.")
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_prompt", "prompt", "Field 'prompt' is required.")
	}
	if len(req.Image) == 0 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_image", "image", "Form field 'image' is required.")
	}
	if req.N < 1 || req.N > 10 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_n", "n", "Field 'n' must be between 1 and 10.")
	}
	switch req.ResponseFormat {
	case "", "url", "b64_json":
	default:
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_response_format", "response_format", "Field 'response_format' must be 'url' or 'b64_json'.")
	}
	if req.ResponseFormat == "" {
		req.ResponseFormat = "url"
	}

	imageFormat := fileFormatFromName(req.ImageFilename)
	if imageFormat != "png" && imageFormat != "jpg" && imageFormat != "jpeg" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_image_format", "image", "Field 'image' must be a PNG or JPEG file.")
	}
	if len(req.Mask) > 0 && fileFormatFromName(req.MaskFilename) != "png" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_mask_format", "mask", "Field 'mask' must be a PNG file.")
	}
	return nil
}
