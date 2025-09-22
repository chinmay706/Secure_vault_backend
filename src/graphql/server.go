package graphql

import (
	"log"
	"net/http"
	"securevault-backend/src/graphql/graph"
	"securevault-backend/src/graphql/middleware"
	"securevault-backend/src/services"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/vektah/gqlparser/v2/ast"
)

// NewGraphQLHandler creates the GraphQL HTTP handler with services
func NewGraphQLHandler(authService *services.AuthService, fileService *services.FileService, folderService *services.FolderService, statsService *services.StatsService, storageService *services.StorageService) http.Handler {
	log.Printf("[GQL-SERVER] Initializing GraphQL handler with services")
	
	// Create resolver with services
	resolver := graph.NewResolver(authService, fileService, folderService, statsService, storageService)

	// Create GraphQL server
	srv := handler.New(graph.NewExecutableSchema(graph.Config{Resolvers: resolver}))
	log.Printf("[GQL-SERVER] GraphQL server created successfully")

	// Add transports
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})

	// Set query cache
	srv.SetQueryCache(lru.New[*ast.QueryDocument](1000))

	// Add extensions
	srv.Use(extension.Introspection{})
	srv.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New[string](100),
	})

	// Wrap with logging and auth middleware
	return loggingMiddleware(middleware.AuthMiddleware(authService)(srv))
}

// loggingMiddleware logs GraphQL requests
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			log.Printf("[GQL-REQUEST] %s %s - Content-Length: %d", r.Method, r.URL.Path, r.ContentLength)
		}
		next.ServeHTTP(w, r)
	})
}

// NewPlaygroundHandler creates the GraphQL Playground handler
func NewPlaygroundHandler(graphqlEndpoint string) http.Handler {
	return playground.Handler("GraphQL Playground", graphqlEndpoint)
}
