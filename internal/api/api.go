package api

import (
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	fiberLogger "github.com/gofiber/fiber/v2/middleware/logger"
	fiberRecover "github.com/gofiber/fiber/v2/middleware/recover"
	"go.uber.org/zap"

	"github.com/duyike/greddit/internal/api/handler"
	"github.com/duyike/greddit/internal/api/middleware"
	internalDB "github.com/duyike/greddit/internal/pkg/db"
	"github.com/duyike/greddit/internal/post"
	"github.com/duyike/greddit/internal/user"
)

var (
	logger, _ = zap.NewProduction(zap.Fields(zap.String("type", "api")))
)

type App interface {
	GracefulShutdown(shutdown chan struct{})
	Listen(addr string) error
	FiberApp() *fiber.App
}

type app struct {
	*fiber.App
	shutdowns []func() error
}

func NewApp() (App, error) {
	var (
		shutdowns []func() error
	)
	rand.Seed(time.Now().UnixNano())
	db, err := internalDB.NewDb()
	if err != nil {
		return nil, err
	}
	//sqlDB, err := db.DB()
	//if err != nil {
	//	return nil, err
	//}
	//shutdowns = append(shutdowns, sqlDB.Close)

	userRepository := user.NewRepository(db)
	postRepository := post.NewRepository(db)

	userApp := user.NewApp(userRepository)
	postApp := post.NewApp(postRepository, userApp)

	fiberApp := fiber.New(fiber.Config{ErrorHandler: middleware.NewBizErrorHandler()})
	fiberApp.Use(fiberRecover.New())
	fiberApp.Use(cors.New(cors.Config{

		AllowHeaders: strings.Join([]string{
			fiber.HeaderOrigin,
			fiber.HeaderContentLength,
			fiber.HeaderContentType,
		}, ","),
		AllowCredentials: true,
	}))
	fiberApp.Use(fiberLogger.New())
	fiberApp.Get("/health", func(ctx *fiber.Ctx) error {
		return ctx.Status(fiber.StatusOK).SendString("ok")
	})
	fiberApp.Mount("/user", handler.NewUserHandler(userApp).App)
	fiberApp.Mount("/post", handler.NewPostHandler(postApp).App)
	return app{
		App:       fiberApp,
		shutdowns: shutdowns,
	}, nil
}

func (a app) GracefulShutdown(shutdown chan struct{}) {
	var (
		sigint = make(chan os.Signal, 1)
	)

	signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
	<-sigint

	logger.Info("shutting down server gracefully")
	if err := a.Shutdown(); err != nil {
		logger.Fatal("shutdown error", zap.Error(err))
	}
	for i := range a.shutdowns {
		err := a.shutdowns[i]()
		if err != nil {
			logger.Error("sub shutdown error", zap.Any("shutdowns", a.shutdowns[i]))
		}
	}
	close(shutdown)
}

func (a app) Listen(addr string) error {
	return a.App.Listen(addr)
}

func (a app) FiberApp() *fiber.App {
	return a.App
}
