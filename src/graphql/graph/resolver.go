package graph

import (
	"securevault-backend/src/services"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.

type Resolver struct {
	AuthService    *services.AuthService
	FileService    *services.FileService
	FolderService  *services.FolderService
	StatsService   *services.StatsService
	StorageService *services.StorageService
	AiTagService   *services.AiTagService
}

// NewResolver creates a new resolver with service dependencies
func NewResolver(
	authService *services.AuthService,
	fileService *services.FileService,
	folderService *services.FolderService,
	statsService *services.StatsService,
	storageService *services.StorageService,
	aiTagService *services.AiTagService,
) *Resolver {
	return &Resolver{
		AuthService:    authService,
		FileService:    fileService,
		FolderService:  folderService,
		StatsService:   statsService,
		StorageService: storageService,
		AiTagService:   aiTagService,
	}
}
