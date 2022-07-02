package infra

import (
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/umalmyha/customers/internal/domain/auth"
	"github.com/umalmyha/customers/internal/handlers"
	"github.com/umalmyha/customers/internal/middleware"
	"github.com/umalmyha/customers/internal/repository"
	"github.com/umalmyha/customers/internal/service"
	"github.com/umalmyha/customers/pkg/db/transactor"
	"go.mongodb.org/mongo-driver/mongo"
)

func Router(pgPool *pgxpool.Pool, mongoClient *mongo.Client, authCfg AuthCfg) *echo.Echo {
	e := echo.New()

	e.HTTPErrorHandler = func(err error, c echo.Context) {
		c.Logger().Error(err.Error())
		e.DefaultHTTPErrorHandler(err, c)
	}

	// Transactors
	trx := transactor.NewPgxTransactor(pgPool)

	// Configs
	jwtCfg := authCfg.JwtCfg
	rfrTokenCfg := authCfg.RefreshTokenCfg

	// Extra functionality
	jwtIssuer := auth.NewJwtIssuer(jwtCfg.Issuer, jwtCfg.SigningMethod, jwtCfg.TimeToLive, jwtCfg.PrivateKey)
	jwtValidator := auth.NewJwtValidator(jwtCfg.SigningMethod, jwtCfg.PublicKey)
	rfrTokenIssuer := auth.NewRefreshTokenIssuer(rfrTokenCfg.MaxCount, rfrTokenCfg.TimeToLive)

	// Middleware
	authorizeMw := middleware.Authorize(jwtValidator)

	// Repositories
	userRepo := repository.NewPostgresUserRepository(trx)
	rfrTokenRepo := repository.NewPostgresRefreshTokenRepository(trx)
	pgCustRepo := repository.NewPostgresCustomerRepository(pgPool)
	mongoCustRepo := repository.NewMongoCustomerRepository(mongoClient)

	// Services
	authSrv := service.NewAuthService(jwtIssuer, rfrTokenIssuer, userRepo, rfrTokenRepo)
	custSrvV1 := service.NewCustomerService(pgCustRepo)
	custSrvV2 := service.NewCustomerService(mongoCustRepo)

	// Handlers
	authHandler := handlers.NewAuthHandler(trx, authSrv, handlers.AuthCfg{Https: authCfg.Https, RefreshTokenCookie: authCfg.RefreshTokenCfg.CookieName})
	custHandlerV1 := handlers.NewCustomerHandler(custSrvV1)
	custHandlerV2 := handlers.NewCustomerHandler(custSrvV2)

	// API routes
	api := e.Group("/api")

	// auth
	authApi := api.Group("/auth")
	authApi.POST("/signup", authHandler.Signup)
	authApi.POST("/login", authHandler.Login)
	authApi.POST("/logout", authHandler.Logout)
	authApi.POST("/refresh", authHandler.Refresh)

	// customers v1
	customersApiV1 := api.Group("/v1/customers", authorizeMw)
	customersApiV1.GET("", custHandlerV1.GetAll)
	customersApiV1.GET("/:id", custHandlerV1.Get)
	customersApiV1.POST("", custHandlerV1.Post)
	customersApiV1.PUT("/:id", custHandlerV1.Put)
	customersApiV1.DELETE("/:id", custHandlerV1.DeleteById)

	// customers v2
	customersApiV2 := api.Group("/v2/customers", authorizeMw)
	customersApiV2.GET("", custHandlerV2.GetAll)
	customersApiV2.GET("/:id", custHandlerV2.Get)
	customersApiV2.POST("", custHandlerV2.Post)
	customersApiV2.PUT("/:id", custHandlerV2.Put)
	customersApiV2.DELETE("/:id", custHandlerV2.DeleteById)

	return e
}
