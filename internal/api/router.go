package api

import (
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func SetupRouter(h *Handler, apiKey string) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(CORSMiddleware())
	r.Use(LoggingMiddleware())

	// Swagger
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	v1 := r.Group("/api/v1")
	{
		v1.GET("/health", h.Health)

		jobs := v1.Group("/jobs")
		// Optional API-key gate (health stays public for Railway healthchecks).
		if apiKey != "" {
			jobs.Use(APIKeyAuth(apiKey))
		}
		{
			jobs.POST("", h.CreateJob)
			jobs.GET("", h.ListJobs)
			jobs.GET("/:id", h.GetJob)
			jobs.GET("/:id/events", h.CheckJobAndStreamEvents)
			jobs.GET("/:id/download", h.DownloadJob)
			jobs.DELETE("/:id", h.DeleteJob)
		}
	}

	return r
}
