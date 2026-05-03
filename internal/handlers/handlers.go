package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"proxy-convert/internal/service"

	"github.com/gin-gonic/gin"
)

type ImportRequest struct {
	URL   string `json:"url"`
	Links string `json:"links"`
}

type Response struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func RegisterRoutes(router *gin.Engine, linkService *service.LinkService, verifierService *service.VerifierService, extractorService *service.ExtractorService, clashService *service.ClashService) {
	router.GET("/", func(c *gin.Context) {
		c.File("templates/index.html")
	})

	router.GET("/icon.png", func(c *gin.Context) {
		c.File("templates/icon.png")
	})

	router.GET("/api/hello", func(c *gin.Context) {
		c.JSON(http.StatusOK, Response{
			Success: true,
			Message: "Hello, World!",
		})
	})

	router.POST("/api/links/import", func(c *gin.Context) {
		var req ImportRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, Response{
				Success: false,
				Message: "Invalid request",
			})
			return
		}

		if req.URL == "" {
			c.JSON(http.StatusBadRequest, Response{
				Success: false,
				Message: "URL is required",
			})
			return
		}

		imported, existing, err := extractorService.ImportFromURL(req.URL)
		if err != nil {
			c.JSON(http.StatusBadRequest, Response{
				Success: false,
				Message: err.Error(),
			})
			return
		}

		totalLinks := imported + existing
		c.JSON(http.StatusOK, Response{
			Success: true,
			Message: "Links imported successfully",
			Data: map[string]int{
				"total_links": totalLinks,
				"imported":    imported,
				"existing":    existing,
			},
		})
	})

	router.POST("/api/links/import-text", func(c *gin.Context) {
		var req ImportRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, Response{
				Success: false,
				Message: "Invalid request",
			})
			return
		}

		if req.Links == "" {
			c.JSON(http.StatusBadRequest, Response{
				Success: false,
				Message: "Links text is required",
			})
			return
		}

		imported, existing, err := extractorService.ImportFromText(req.Links)
		if err != nil {
			c.JSON(http.StatusBadRequest, Response{
				Success: false,
				Message: err.Error(),
			})
			return
		}

		totalLinks := imported + existing
		c.JSON(http.StatusOK, Response{
			Success: true,
			Message: "Links imported successfully",
			Data: map[string]int{
				"total_links": totalLinks,
				"imported":    imported,
				"existing":    existing,
			},
		})
	})

	router.GET("/api/links/verify", func(c *gin.Context) {
		go func() {
			if err := verifierService.VerifyLinks(); err != nil {
				c.Error(err)
			}
		}()

		c.JSON(http.StatusOK, Response{
			Success: true,
			Message: "Link verification started in background",
			Data: map[string]interface{}{
				"status": "processing",
				"info":   "Verification results will be saved to database",
			},
		})
	})

	router.GET("/api/links", func(c *gin.Context) {
		status := c.Query("status")
		var statuses []int
		if status != "" {
			s := 0
			if err := c.BindQuery(&s); err == nil {
				statuses = []int{s}
			}
		}

		links, err := linkService.GetAllLinks(statuses, 0, 0)
		if err != nil {
			c.JSON(http.StatusInternalServerError, Response{
				Success: false,
				Message: err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, Response{
			Success: true,
			Message: "Links retrieved successfully",
			Data:    links,
		})
	})

	router.GET("/api/links/:id", func(c *gin.Context) {
		id := 0
		if err := c.ShouldBindUri(&map[string]int{"id": id}); err != nil {
			c.JSON(http.StatusBadRequest, Response{
				Success: false,
				Message: "Invalid link ID",
			})
			return
		}

		link, err := linkService.GetLink(id)
		if err != nil {
			c.JSON(http.StatusNotFound, Response{
				Success: false,
				Message: "Link not found",
			})
			return
		}

		c.JSON(http.StatusOK, Response{
			Success: true,
			Message: "Link retrieved successfully",
			Data:    link,
		})
	})

	router.PUT("/api/links/:id", func(c *gin.Context) {
		id := 0
		if err := c.ShouldBindUri(&map[string]int{"id": id}); err != nil {
			c.JSON(http.StatusBadRequest, Response{
				Success: false,
				Message: "Invalid link ID",
			})
			return
		}

		var req struct {
			Status int `json:"status"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, Response{
				Success: false,
				Message: "Invalid request",
			})
			return
		}

		if _, err := linkService.UpdateLinkStatus(id, req.Status); err != nil {
			c.JSON(http.StatusInternalServerError, Response{
				Success: false,
				Message: err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, Response{
			Success: true,
			Message: "Link updated successfully",
		})
	})

	router.DELETE("/api/links/:id", func(c *gin.Context) {
		id := 0
		if err := c.ShouldBindUri(&map[string]int{"id": id}); err != nil {
			c.JSON(http.StatusBadRequest, Response{
				Success: false,
				Message: "Invalid link ID",
			})
			return
		}

		if _, err := linkService.DeleteLink(id); err != nil {
			c.JSON(http.StatusInternalServerError, Response{
				Success: false,
				Message: err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, Response{
			Success: true,
			Message: "Link deleted successfully",
		})
	})

	router.GET("/api/links/count", func(c *gin.Context) {
		status := c.Query("status")
		var statuses []int
		if status != "" {
			s := 0
			if err := c.BindQuery(&s); err == nil {
				statuses = []int{s}
			}
		}

		count, err := linkService.CountLinks(statuses)
		if err != nil {
			c.JSON(http.StatusInternalServerError, Response{
				Success: false,
				Message: err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, Response{
			Success: true,
			Message: "Links count retrieved successfully",
			Data: map[string]int{
				"count": count,
			},
		})
	})

	router.GET("/api/clash/config", func(c *gin.Context) {
		status := c.Query("status")
		var statuses []int
		if status != "" {
			s := 0
			if err := c.BindQuery(&s); err == nil {
				statuses = []int{s}
			}
		}

		config, err := clashService.BuildClash(statuses)
		if err != nil {
			c.JSON(http.StatusInternalServerError, Response{
				Success: false,
				Message: err.Error(),
			})
			return
		}

		c.Header("Content-Type", "application/json; charset=utf-8")
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		encoder.SetEscapeHTML(false)
		encoder.Encode(config)
		c.String(http.StatusOK, buf.String())
	})

	router.GET("/api/clash/export", func(c *gin.Context) {
		status := c.Query("status")
		var statuses []int
		if status != "" {
			s := 0
			if err := c.BindQuery(&s); err == nil {
				statuses = []int{s}
			}
		}

		config, err := clashService.BuildClash(statuses)
		if err != nil {
			c.JSON(http.StatusInternalServerError, Response{
				Success: false,
				Message: err.Error(),
			})
			return
		}

		c.Header("Content-Type", "application/json; charset=utf-8")
		c.Header("Content-Disposition", "attachment; filename=clash_config.json")
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		encoder.SetEscapeHTML(false)
		encoder.Encode(config)
		c.String(http.StatusOK, buf.String())
	})

	router.GET("/api/clash", func(c *gin.Context) {
		status := c.Query("status")
		var statuses []int
		if status != "" {
			s := 0
			if err := c.BindQuery(&s); err == nil {
				statuses = []int{s}
			}
		} else {
			// 缺省值为 1
			statuses = []int{1}
		}

		yamlData, err := clashService.ExportClashConfigYAML(statuses)
		if err != nil {
			c.JSON(http.StatusInternalServerError, Response{
				Success: false,
				Message: err.Error(),
			})
			return
		}

		c.Header("Content-Type", "text/yaml; charset=utf-8")
		c.Data(http.StatusOK, "text/yaml", yamlData)
	})

	router.GET("/api/v2ray", func(c *gin.Context) {
		status := c.Query("status")
		var statuses []int
		if status != "" {
			s := 0
			if err := c.BindQuery(&s); err == nil {
				statuses = []int{s}
			}
		} else {
			// 缺省值为 1
			statuses = []int{1}
		}

		links, err := linkService.GetAllLinks(statuses, 0, 0)
		if err != nil {
			c.JSON(http.StatusInternalServerError, Response{
				Success: false,
				Message: err.Error(),
			})
			return
		}

		// 构建响应内容，每一个 link 一行
		var responseContent string
		for _, link := range links {
			responseContent += link.Link + "\n"
		}

		c.Header("Content-Type", "text/plain; charset=utf-8")
		c.String(http.StatusOK, responseContent)
	})
}
