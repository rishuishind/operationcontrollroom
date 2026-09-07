// Package api exposes the HTTP handlers for the Go backend, built on Gin.
package api

import (
	"database/sql"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/rishabh/operation-control-room/internal/recommend"
)

type Server struct {
	DB      *sql.DB
	Weather recommend.WeatherSource
	// Now lets tests/demo override the clock (e.g. ?at=2024-01-05T17:00:00Z).
	Now func() time.Time
}

func New(conn *sql.DB, ws recommend.WeatherSource) *Server {
	return &Server{DB: conn, Weather: ws, Now: time.Now}
}

func (s *Server) Routes() *gin.Engine {
	engine := gin.Default()
	engine.Use(cors())

	engine.GET("/api/health", s.handleHealth)
	engine.GET("/api/recommendation", s.handleRecommendation)

	return engine
}

func (s *Server) handleHealth(c *gin.Context) {
	c.String(http.StatusOK, "ok")
}

func (s *Server) handleRecommendation(c *gin.Context) {
	now := s.Now()
	if at := c.Query("at"); at != "" {
		parsed, err := time.Parse(time.RFC3339, at)
		if err != nil {
			c.String(http.StatusBadRequest, "invalid 'at' timestamp, expected RFC3339")
			return
		}
		now = parsed
	}

	weatherSource := s.Weather
	if rainArea := c.Query("demo_rain_area"); rainArea != "" {
		log.Printf("DEMO MODE: simulating heavy rain for community area id=%s (requested via ?demo_rain_area)", rainArea)
		weatherSource = withDemoRain(s.Weather, rainArea)
	}

	rec, err := recommend.Build(c.Request.Context(), s.DB, weatherSource, now)
	if err != nil {
		log.Printf("build recommendation: %v", err)
		c.String(http.StatusInternalServerError, "failed to build recommendation")
		return
	}

	c.JSON(http.StatusOK, rec)
}

func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
