package services

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	"securevault-backend/src/models"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// FolderService handles folder-related operations
type FolderService struct {
	db          *sql.DB
	fileService *FileService // For proper file deletion with blob cleanup
}

// NewFolderService creates a new folder service
func NewFolderService(db *sql.DB) *FolderService {
	return &FolderService{
		db:          db,
		fileService: nil, // Set later via SetFileService to avoid circular dependency
	}
}

// SetFileService sets the file service for proper file deletion
func (fs *FolderService) SetFileService(fileService *FileService) {
	fs.fileService = fileService
}

// ListFoldersOnly returns only child folders for the given parent (simple version without files)
func (fs *FolderService) ListFoldersOnly(ownerID uuid.UUID, parentID *uuid.UUID) ([]*models.Folder, error) {
	log.Printf("[FOLDER-SERVICE] ListFoldersOnly called - ownerID: %s, parentID: %v", ownerID, parentID)
	
	var folderRows *sql.Rows
	var err error
	
	if parentID == nil {
		// Root level folders - use hardcoded query to bypass prepared statement issues
		log.Printf("[FOLDER-SERVICE] Querying root level folders for ownerID: %s", ownerID)
		log.Printf("[FOLDER-SERVICE] SQL: SELECT folders WHERE owner_id = %s AND parent_id IS NULL", ownerID)
		query := fmt.Sprintf(`
			SELECT root_f.id, root_f.owner_id, root_f.name, root_f.parent_id, root_f.created_at, root_f.updated_at
			FROM folders root_f 
			WHERE root_f.owner_id = '%s' AND root_f.parent_id IS NULL
			ORDER BY root_f.name ASC
		`, ownerID.String())
		folderRows, err = fs.db.Query(query)
	} else {
		log.Printf("[FOLDER-SERVICE] Querying child folders for parentID: %s", *parentID)
		log.Printf("[FOLDER-SERVICE] SQL: SELECT folders WHERE owner_id = %s AND parent_id = %s", ownerID, *parentID)
		query := fmt.Sprintf(`
			SELECT child_f.id, child_f.owner_id, child_f.name, child_f.parent_id, child_f.created_at, child_f.updated_at
			FROM folders child_f 
			WHERE child_f.owner_id = '%s' AND child_f.parent_id = '%s'
			ORDER BY child_f.name ASC
		`, ownerID.String(), parentID.String())
		folderRows, err = fs.db.Query(query)
	}
	
	if err != nil {
		log.Printf("[FOLDER-SERVICE] ListFoldersOnly query failed: %v", err)
		return nil, fmt.Errorf("failed to get folders: %w", err)
	}
	log.Printf("[FOLDER-SERVICE] ListFoldersOnly query executed successfully")
	defer folderRows.Close()

	var folders []*models.Folder
	for folderRows.Next() {
		folder := &models.Folder{}
		err := folderRows.Scan(
			&folder.ID, &folder.OwnerID, &folder.Name,
			&folder.ParentID, &folder.CreatedAt, &folder.UpdatedAt,
		)
		if err != nil {
			log.Printf("[FOLDER-SERVICE] Failed to scan folder: %v", err)
			return nil, fmt.Errorf("failed to scan folder: %w", err)
		}
		folders = append(folders, folder)
	}

	if err := folderRows.Err(); err != nil {
		log.Printf("[FOLDER-SERVICE] Error reading folders: %v", err)
		return nil, fmt.Errorf("error reading folders: %w", err)
	}
	
	log.Printf("[FOLDER-SERVICE] ListFoldersOnly returning %d folders", len(folders))
	return folders, nil
}

// CreateFolder creates a new folder with validation
func (fs *FolderService) CreateFolder(ownerID uuid.UUID, name string, parentID *uuid.UUID) (*models.Folder, error) {
	// Validate name
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("folder name cannot be empty")
	}

	// Check if parent exists and belongs to same owner (if parentID is provided)
	if parentID != nil {
		var parentOwnerID uuid.UUID
		err := fs.db.QueryRow(
			"SELECT owner_id FROM folders WHERE id = $1",
			*parentID,
		).Scan(&parentOwnerID)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, fmt.Errorf("parent folder not found")
			}
			return nil, fmt.Errorf("failed to validate parent folder: %w", err)
		}
		
		if parentOwnerID != ownerID {
			return nil, fmt.Errorf("parent folder does not belong to the same owner")
		}
	}

	// Check depth limit (max 5)
	if parentID != nil {
		depth, err := fs.ComputeDepth(*parentID)
		if err != nil {
			return nil, fmt.Errorf("failed to compute parent depth: %w", err)
		}
		if depth >= 5 {
			return nil, fmt.Errorf("maximum folder depth (5) exceeded")
		}
	}

	// Create folder
	folder := models.NewFolder(ownerID, name, parentID)
	
	// Insert into database
	_, err := fs.db.Exec(`
		INSERT INTO folders (id, owner_id, name, parent_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, folder.ID, folder.OwnerID, folder.Name, folder.ParentID, folder.CreatedAt, folder.UpdatedAt)
	
	if err != nil {
		// Check for unique constraint violation
		if strings.Contains(err.Error(), "folders_owner_parent_name_unique") {
			return nil, fmt.Errorf("a folder with this name already exists in the same location")
		}
		return nil, fmt.Errorf("failed to create folder: %w", err)
	}

	return folder, nil
}

// RenameFolder renames an existing folder with validation
func (fs *FolderService) RenameFolder(ownerID, folderID uuid.UUID, newName string) (*models.Folder, error) {
	// Validate name
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return nil, fmt.Errorf("folder name cannot be empty")
	}

	// Get current folder to verify ownership and get parent_id
	var folder models.Folder
	err := fs.db.QueryRow(`
		SELECT id, owner_id, name, parent_id, created_at, updated_at
		FROM folders 
		WHERE id = $1 AND owner_id = $2
	`, folderID, ownerID).Scan(
		&folder.ID, &folder.OwnerID, &folder.Name, 
		&folder.ParentID, &folder.CreatedAt, &folder.UpdatedAt,
	)
	
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("folder not found")
		}
		return nil, fmt.Errorf("failed to get folder: %w", err)
	}

	// Update name and timestamp
	folder.SetName(newName)

	// Update in database
	_, err = fs.db.Exec(`
		UPDATE folders 
		SET name = $1, updated_at = $2
		WHERE id = $3 AND owner_id = $4
	`, folder.Name, folder.UpdatedAt, folder.ID, ownerID)
	
	if err != nil {
		// Check for unique constraint violation
		if strings.Contains(err.Error(), "folders_owner_parent_name_unique") {
			return nil, fmt.Errorf("a folder with this name already exists in the same location")
		}
		return nil, fmt.Errorf("failed to rename folder: %w", err)
	}

	return &folder, nil
}

// MoveFolder moves a folder to a new parent with validation
func (fs *FolderService) MoveFolder(ownerID, folderID uuid.UUID, newParentID *uuid.UUID) (*models.Folder, error) {
	// Get current folder to verify ownership
	var folder models.Folder
	err := fs.db.QueryRow(`
		SELECT id, owner_id, name, parent_id, created_at, updated_at
		FROM folders 
		WHERE id = $1 AND owner_id = $2
	`, folderID, ownerID).Scan(
		&folder.ID, &folder.OwnerID, &folder.Name, 
		&folder.ParentID, &folder.CreatedAt, &folder.UpdatedAt,
	)
	
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("folder not found")
		}
		return nil, fmt.Errorf("failed to get folder: %w", err)
	}

	// Check if parent exists and belongs to same owner (if newParentID is provided)
	if newParentID != nil {
		// Check if trying to move to itself
		if *newParentID == folderID {
			return nil, fmt.Errorf("cannot move folder to itself")
		}

		// Check if new parent exists and belongs to same owner
		var parentOwnerID uuid.UUID
		err := fs.db.QueryRow(
			"SELECT owner_id FROM folders WHERE id = $1",
			*newParentID,
		).Scan(&parentOwnerID)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, fmt.Errorf("target parent folder not found")
			}
			return nil, fmt.Errorf("failed to validate parent folder: %w", err)
		}
		
		if parentOwnerID != ownerID {
			return nil, fmt.Errorf("target parent folder does not belong to the same owner")
		}

		// Check for cycles (prevent moving to descendant)
		isDescendant, err := fs.IsDescendant(folderID, *newParentID)
		if err != nil {
			return nil, fmt.Errorf("failed to check for cycles: %w", err)
		}
		if isDescendant {
			return nil, fmt.Errorf("cannot move folder to its descendant")
		}

		// Check depth limit after move
		parentDepth, err := fs.ComputeDepth(*newParentID)
		if err != nil {
			return nil, fmt.Errorf("failed to compute parent depth: %w", err)
		}
		
		// Get subtree depth of folder being moved
		subtreeDepth, err := fs.ComputeSubtreeDepth(folderID)
		if err != nil {
			return nil, fmt.Errorf("failed to compute subtree depth: %w", err)
		}
		
		// New depth would be parent depth + 1 + subtree depth
		if parentDepth+1+subtreeDepth > 5 {
			return nil, fmt.Errorf("move would exceed maximum folder depth (5)")
		}
	}

	// Update parent and timestamp
	folder.SetParent(newParentID)

	// Update in database
	_, err = fs.db.Exec(`
		UPDATE folders 
		SET parent_id = $1, updated_at = $2
		WHERE id = $3 AND owner_id = $4
	`, folder.ParentID, folder.UpdatedAt, folder.ID, ownerID)
	
	if err != nil {
		// Check for unique constraint violation
		if strings.Contains(err.Error(), "folders_owner_parent_name_unique") {
			return nil, fmt.Errorf("a folder with this name already exists in the target location")
		}
		return nil, fmt.Errorf("failed to move folder: %w", err)
	}

	return &folder, nil
}

// DeleteFolder deletes a folder and optionally its contents
func (fs *FolderService) DeleteFolder(ownerID, folderID uuid.UUID, recursive bool) error {
	// Check if folder exists and is owned by user
	var exists bool
	err := fs.db.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM folders WHERE id = $1 AND owner_id = $2)",
		folderID, ownerID,
	).Scan(&exists)
	
	if err != nil {
		return fmt.Errorf("failed to check folder existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("folder not found")
	}

	if !recursive {
		// Check if folder has children (folders or files)
		var hasChildren bool
		err = fs.db.QueryRow(`
			SELECT EXISTS(
				SELECT 1 FROM folders WHERE parent_id = $1
				UNION
				SELECT 1 FROM files WHERE folder_id = $1
			)
		`, folderID).Scan(&hasChildren)
		
		if err != nil {
			return fmt.Errorf("failed to check for children: %w", err)
		}
		if hasChildren {
			return fmt.Errorf("folder is not empty, use recursive delete or move contents first")
		}
	}

	// Start transaction for atomic deletion
	tx, err := fs.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if recursive {
		// First delete all files in the folder tree to ensure proper blob cleanup
		if fs.fileService != nil {
			err = fs.deleteFilesInFolderTree(ownerID, folderID)
			if err != nil {
				return fmt.Errorf("failed to delete files in folder tree: %w", err)
			}
		}
		
		// Delete all descendant folders recursively
		_, err = tx.Exec(`
			WITH RECURSIVE folder_tree AS (
				-- Base case: the folder to delete
				SELECT id FROM folders WHERE id = $1 AND owner_id = $2
				UNION ALL
				-- Recursive case: child folders
				SELECT f.id FROM folders f
				INNER JOIN folder_tree ft ON f.parent_id = ft.id
			)
			DELETE FROM folders WHERE id IN (SELECT id FROM folder_tree)
		`, folderID, ownerID)
		
		if err != nil {
			return fmt.Errorf("failed to delete folder tree: %w", err)
		}
	} else {
		// Delete just the folder (should be empty)
		_, err = tx.Exec("DELETE FROM folders WHERE id = $1 AND owner_id = $2", folderID, ownerID)
		if err != nil {
			return fmt.Errorf("failed to delete folder: %w", err)
		}
	}

	// Commit transaction
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit deletion: %w", err)
	}

	return nil
}

// GetFolderByID retrieves a folder by ID for the given owner
func (fs *FolderService) GetFolderByID(ownerID, folderID uuid.UUID) (*models.Folder, error) {
	log.Printf("[FOLDER-SERVICE] GetFolderByID called - ownerID: %s, folderID: %s", ownerID, folderID)
	
	// Use hardcoded query to avoid prepared statement issues
	query := fmt.Sprintf(`
		SELECT folder_single.id, folder_single.owner_id, folder_single.name, folder_single.parent_id, folder_single.created_at, folder_single.updated_at
		FROM folders folder_single 
		WHERE folder_single.id = '%s' AND folder_single.owner_id = '%s'
	`, folderID.String(), ownerID.String())
	
	log.Printf("[FOLDER-SERVICE] GetFolderByID query: %s", query)
	
	var folder models.Folder
	err := fs.db.QueryRow(query).Scan(
		&folder.ID, &folder.OwnerID, &folder.Name, 
		&folder.ParentID, &folder.CreatedAt, &folder.UpdatedAt,
	)
	
	if err != nil {
		log.Printf("[FOLDER-SERVICE] GetFolderByID query failed: %v", err)
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("folder not found")
		}
		return nil, fmt.Errorf("failed to get folder: %w", err)
	}

	log.Printf("[FOLDER-SERVICE] GetFolderByID success - folder: %s", folder.Name)
	return &folder, nil
}

// GetBreadcrumbs returns the path from root to the given folder
func (fs *FolderService) GetBreadcrumbs(ownerID, folderID uuid.UUID) ([]*models.Folder, error) {
	log.Printf("[FOLDER-SERVICE] GetBreadcrumbs called - ownerID: %s, folderID: %s", ownerID, folderID)
	
	// Use hardcoded recursive CTE to avoid prepared statement issues
	query := fmt.Sprintf(`
		WITH RECURSIVE folder_path AS (
			-- Base case: the target folder
			SELECT breadcrumb_f.id, breadcrumb_f.owner_id, breadcrumb_f.name, breadcrumb_f.parent_id, breadcrumb_f.created_at, breadcrumb_f.updated_at, 0 as level
			FROM folders breadcrumb_f 
			WHERE breadcrumb_f.id = '%s' AND breadcrumb_f.owner_id = '%s'
			
			UNION ALL
			
			-- Recursive case: parent folders
			SELECT parent_f.id, parent_f.owner_id, parent_f.name, parent_f.parent_id, parent_f.created_at, parent_f.updated_at, fp.level + 1
			FROM folders parent_f
			INNER JOIN folder_path fp ON parent_f.id = fp.parent_id
		)
		SELECT breadcrumb_result.id, breadcrumb_result.owner_id, breadcrumb_result.name, breadcrumb_result.parent_id, breadcrumb_result.created_at, breadcrumb_result.updated_at
		FROM folder_path breadcrumb_result
		ORDER BY level DESC  -- Root first, target folder last
	`, folderID.String(), ownerID.String())
	
	log.Printf("[FOLDER-SERVICE] GetBreadcrumbs query: %s", query)
	rows, err := fs.db.Query(query)
	
	if err != nil {
		log.Printf("[FOLDER-SERVICE] GetBreadcrumbs query failed: %v", err)
		return nil, fmt.Errorf("failed to get breadcrumbs: %w", err)
	}
	defer rows.Close()

	var breadcrumbs []*models.Folder
	for rows.Next() {
		folder := &models.Folder{}
		err := rows.Scan(
			&folder.ID, &folder.OwnerID, &folder.Name,
			&folder.ParentID, &folder.CreatedAt, &folder.UpdatedAt,
		)
		if err != nil {
			log.Printf("[FOLDER-SERVICE] GetBreadcrumbs scan failed: %v", err)
			return nil, fmt.Errorf("failed to scan breadcrumb: %w", err)
		}
		breadcrumbs = append(breadcrumbs, folder)
	}

	if err := rows.Err(); err != nil {
		log.Printf("[FOLDER-SERVICE] GetBreadcrumbs rows error: %v", err)
		return nil, fmt.Errorf("error reading breadcrumbs: %w", err)
	}

	if len(breadcrumbs) == 0 {
		log.Printf("[FOLDER-SERVICE] GetBreadcrumbs: no breadcrumbs found")
		return nil, fmt.Errorf("folder not found")
	}

	log.Printf("[FOLDER-SERVICE] GetBreadcrumbs success - breadcrumbs count: %d", len(breadcrumbs))
	return breadcrumbs, nil
}

// ComputeDepth calculates the depth of a folder (0 = root level)
func (fs *FolderService) ComputeDepth(folderID uuid.UUID) (int, error) {
	var depth int
	err := fs.db.QueryRow(`
		WITH RECURSIVE folder_depth AS (
			-- Base case: the target folder
			SELECT id, parent_id, 0 as depth
			FROM folders 
			WHERE id = $1
			
			UNION ALL
			
			-- Recursive case: parent folders
			SELECT f.id, f.parent_id, fd.depth + 1
			FROM folders f
			INNER JOIN folder_depth fd ON f.id = fd.parent_id
		)
		SELECT MAX(depth) FROM folder_depth
	`, folderID).Scan(&depth)
	
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("folder not found")
		}
		return 0, fmt.Errorf("failed to compute depth: %w", err)
	}

	return depth, nil
}

// ComputeSubtreeDepth calculates the maximum depth within a folder's subtree
func (fs *FolderService) ComputeSubtreeDepth(folderID uuid.UUID) (int, error) {
	var maxDepth sql.NullInt32
	err := fs.db.QueryRow(`
		WITH RECURSIVE subtree AS (
			-- Base case: the folder itself
			SELECT id, parent_id, 0 as depth
			FROM folders 
			WHERE id = $1
			
			UNION ALL
			
			-- Recursive case: descendant folders
			SELECT f.id, f.parent_id, s.depth + 1
			FROM folders f
			INNER JOIN subtree s ON f.parent_id = s.id
		)
		SELECT MAX(depth) FROM subtree
	`, folderID).Scan(&maxDepth)
	
	if err != nil {
		return 0, fmt.Errorf("failed to compute subtree depth: %w", err)
	}

	if !maxDepth.Valid {
		return 0, nil // No descendants
	}

	return int(maxDepth.Int32), nil
}

// IsDescendant checks if folder A is a descendant of folder B
func (fs *FolderService) IsDescendant(ancestorID, descendantID uuid.UUID) (bool, error) {
	var exists bool
	err := fs.db.QueryRow(`
		WITH RECURSIVE folder_ancestors AS (
			-- Base case: the potential descendant
			SELECT id, parent_id
			FROM folders 
			WHERE id = $1
			
			UNION ALL
			
			-- Recursive case: parent folders
			SELECT f.id, f.parent_id
			FROM folders f
			INNER JOIN folder_ancestors fa ON f.id = fa.parent_id
		)
		SELECT EXISTS(
			SELECT 1 FROM folder_ancestors WHERE id = $2
		)
	`, descendantID, ancestorID).Scan(&exists)
	
	if err != nil {
		return false, fmt.Errorf("failed to check descendant relationship: %w", err)
	}

	return exists, nil
}

// FolderChildrenResponse represents the combined response for listing folder children
type FolderChildrenResponse struct {
	Folders []*models.Folder     `json:"folders"`
	Files   []*models.File       `json:"files"`
	FilesPagination FolderPaginationInfo `json:"files_pagination"`
}

// FolderPaginationInfo contains pagination metadata for folder children listing
type FolderPaginationInfo struct {
	Page         int   `json:"page"`
	PageSize     int   `json:"page_size"`
	TotalItems   int64 `json:"total_items"`
	TotalPages   int   `json:"total_pages"`
	HasNext      bool  `json:"has_next"`
	HasPrevious  bool  `json:"has_previous"`
}

// ListChildren returns child folders and files for the given parent folder
func (fs *FolderService) ListChildren(ownerID uuid.UUID, parentID *uuid.UUID, page, pageSize int) (*FolderChildrenResponse, error) {
	log.Printf("[FOLDER-SERVICE] ListChildren called - ownerID: %s, parentID: %v, page: %d, pageSize: %d", ownerID, parentID, page, pageSize)
	
	// Validate pagination parameters
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Get child folders (no pagination for folders)
	var folderRows *sql.Rows
	var err error
	
	if parentID == nil {
		// Root level folders
		log.Printf("[FOLDER-SERVICE] Querying root level folders for ownerID: %s", ownerID)
		folderRows, err = fs.db.Query(`
			SELECT id, owner_id, name, parent_id, created_at, updated_at
			FROM folders 
			WHERE owner_id = $1 AND parent_id IS NULL
			ORDER BY name ASC
		`, ownerID)
	} else {
		// Check if parent folder exists and belongs to owner
		log.Printf("[FOLDER-SERVICE] Checking if parent folder %s exists", *parentID)
		var exists bool
		var folderExists bool
		
		// First check if folder exists at all
		log.Printf("[FOLDER-SERVICE] SQL: SELECT EXISTS(SELECT 1 FROM folders WHERE id = $1) with parentID=%s", *parentID)
		err = fs.db.QueryRow(
			"SELECT EXISTS(SELECT 1 FROM folders WHERE id = $1)",
			*parentID,
		).Scan(&folderExists)
		if err != nil {
			log.Printf("[FOLDER-SERVICE] Folder existence check failed: %v", err)
			return nil, fmt.Errorf("failed to check parent folder existence: %w", err)
		}
		log.Printf("[FOLDER-SERVICE] Folder exists: %t", folderExists)
		
		if !folderExists {
			return nil, fmt.Errorf("folder with id %s does not exist", *parentID)
		}
		
		// Then check if it belongs to the current user
		log.Printf("[FOLDER-SERVICE] SQL: SELECT EXISTS(SELECT 1 FROM folders WHERE id = $1 AND owner_id = $2) with parentID=%s, ownerID=%s", *parentID, ownerID)
		err = fs.db.QueryRow(
			"SELECT EXISTS(SELECT 1 FROM folders WHERE id = $1 AND owner_id = $2)",
			*parentID, ownerID,
		).Scan(&exists)
		if err != nil {
			log.Printf("[FOLDER-SERVICE] Ownership check failed: %v", err)
			return nil, fmt.Errorf("failed to check parent folder ownership: %w", err)
		}
		log.Printf("[FOLDER-SERVICE] User owns folder: %t", exists)
		if !exists {
			return nil, fmt.Errorf("folder with id %s does not belong to user %s", *parentID, ownerID)
		}

		// Get child folders
		log.Printf("[FOLDER-SERVICE] SQL: SELECT child folders WHERE owner_id = $1 AND parent_id = $2 with ownerID=%s, parentID=%s", ownerID, *parentID)
		folderRows, err = fs.db.Query(`
			SELECT id, owner_id, name, parent_id, created_at, updated_at
			FROM folders 
			WHERE owner_id = $1 AND parent_id = $2
			ORDER BY name ASC
		`, ownerID, *parentID)
	}
	
	if err != nil {
		log.Printf("[FOLDER-SERVICE] Child folders query failed: %v", err)
		return nil, fmt.Errorf("failed to get child folders: %w", err)
	}
	log.Printf("[FOLDER-SERVICE] Child folders query executed successfully")
	defer folderRows.Close()

	var folders []*models.Folder
	for folderRows.Next() {
		folder := &models.Folder{}
		err := folderRows.Scan(
			&folder.ID, &folder.OwnerID, &folder.Name,
			&folder.ParentID, &folder.CreatedAt, &folder.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan folder: %w", err)
		}
		folders = append(folders, folder)
	}

	if err := folderRows.Err(); err != nil {
		return nil, fmt.Errorf("error reading folders: %w", err)
	}

	// Get child files with pagination
	var totalFiles int64
	var files []*models.File
	
	// Get total file count for pagination
	var countQuery string
	var countArgs []interface{}
	
	if parentID == nil {
		// Count files in root (folder_id IS NULL)
		countQuery = `SELECT COUNT(*) FROM files WHERE owner_id = $1 AND folder_id IS NULL`
		countArgs = []interface{}{ownerID}
	} else {
		// Count files in specific folder
		countQuery = `SELECT COUNT(*) FROM files WHERE owner_id = $1 AND folder_id = $2`
		countArgs = []interface{}{ownerID, *parentID}
	}
	
	err = fs.db.QueryRow(countQuery, countArgs...).Scan(&totalFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to count files: %w", err)
	}
	
	// Get files with pagination
	offset := (page - 1) * pageSize
	var fileQuery string
	var fileArgs []interface{}
	
	if parentID == nil {
		// Get files in root (folder_id IS NULL)
		fileQuery = `
			SELECT id, owner_id, original_filename, mime_type, size_bytes, 
				   is_public, download_count, tags, folder_id, created_at, updated_at
			FROM files 
			WHERE owner_id = $1 AND folder_id IS NULL
			ORDER BY original_filename ASC
			LIMIT $2 OFFSET $3
		`
		fileArgs = []interface{}{ownerID, pageSize, offset}
	} else {
		// Get files in specific folder
		fileQuery = `
			SELECT id, owner_id, original_filename, mime_type, size_bytes, 
				   is_public, download_count, tags, folder_id, created_at, updated_at
			FROM files 
			WHERE owner_id = $1 AND folder_id = $2
			ORDER BY original_filename ASC
			LIMIT $3 OFFSET $4
		`
		fileArgs = []interface{}{ownerID, *parentID, pageSize, offset}
	}
	
	fileRows, err := fs.db.Query(fileQuery, fileArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to query files: %w", err)
	}
	defer fileRows.Close()
	
	for fileRows.Next() {
		file := &models.File{}
		err := fileRows.Scan(
			&file.ID, &file.OwnerID, &file.OriginalFilename, &file.MimeType,
			&file.SizeBytes, &file.IsPublic, &file.DownloadCount, 
			pq.Array(&file.Tags), &file.FolderID, &file.CreatedAt, &file.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan file: %w", err)
		}
		files = append(files, file)
	}
	
	if err := fileRows.Err(); err != nil {
		return nil, fmt.Errorf("error reading files: %w", err)
	}

	// Calculate pagination info  
	totalPages := int((totalFiles + int64(pageSize) - 1) / int64(pageSize))
	
	// Build response
	response := &FolderChildrenResponse{
		Folders: folders,
		Files:   files,
		FilesPagination: FolderPaginationInfo{
			Page:        page,
			PageSize:    pageSize,
			TotalItems:  totalFiles,
			TotalPages:  totalPages,
			HasNext:     page < totalPages,
			HasPrevious: page > 1,
		},
	}

	log.Printf("[FOLDER-SERVICE] ListChildren completed successfully - returning %d folders, %d files", len(response.Folders), len(response.Files))
	return response, nil
}

// SetFolderPublic manages folder sharing by creating/removing sharelinks
func (fs *FolderService) SetFolderPublic(folderID uuid.UUID, isPublic bool, requesterID uuid.UUID) (*models.Folder, *models.ShareLink, error) {
	// Get the current folder to verify ownership
	folder, err := fs.GetFolderByID(requesterID, folderID)
	if err != nil {
		return nil, nil, err
	}

	var shareLink *models.ShareLink
	if isPublic {
		// Create or enable sharelink when making folder public
		shareLink, err = fs.EnableShareLink(folderID, requesterID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to enable share link: %w", err)
		}
	} else {
		// Disable sharelink when making folder private
		err = fs.DisableShareLink(folderID, requesterID)
		if err != nil && err != ErrShareLinkNotFound {
			// Log the error but don't fail the operation
			// In some cases, the sharelink might not exist yet
		}
	}

	return folder, shareLink, nil
}

// EnableShareLink enables a share link for a folder
func (fs *FolderService) EnableShareLink(folderID uuid.UUID, userID uuid.UUID) (*models.ShareLink, error) {
	// Check ownership
	_, err := fs.GetFolderByID(userID, folderID)
	if err != nil {
		return nil, err
	}

	// Get or create sharelink
	shareLink, err := fs.GetShareLinkByFolderID(folderID)
	if err == ErrShareLinkNotFound {
		// Create new sharelink if it doesn't exist
		return fs.CreateShareLink(folderID, userID)
	}
	if err != nil {
		return nil, err
	}

	// Enable the sharelink
	query := `
		UPDATE sharelinks 
		SET is_active = true
		WHERE folder_id = $1
	`
	
	_, err = fs.db.Exec(query, folderID)
	if err != nil {
		return nil, fmt.Errorf("failed to enable share link: %w", err)
	}

	shareLink.IsActive = true

	return shareLink, nil
}

// DisableShareLink deletes the share link for a folder completely
func (fs *FolderService) DisableShareLink(folderID uuid.UUID, userID uuid.UUID) error {
	// Check ownership
	_, err := fs.GetFolderByID(userID, folderID)
	if err != nil {
		return err
	}

	// Delete the sharelink completely
	query := `DELETE FROM sharelinks WHERE folder_id = $1`
	
	result, err := fs.db.Exec(query, folderID)
	if err != nil {
		return fmt.Errorf("failed to delete share link: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrShareLinkNotFound
	}

	log.Printf("Successfully deleted sharelink for folder %s", folderID)
	return nil
}

// HasShareLink checks if a folder has any sharelinks and returns the token if exists
func (fs *FolderService) HasShareLink(folderID uuid.UUID, userID uuid.UUID) (bool, *string, error) {
	// Verify user has access to the folder
	_, err := fs.GetFolderByID(userID, folderID)
	if err != nil {
		return false, nil, err
	}

	// Check if sharelink exists for this folder and get the token
	query := `SELECT token FROM sharelinks WHERE folder_id = $1 LIMIT 1`
	
	var token string
	err = fs.db.QueryRow(query, folderID).Scan(&token)
	if err != nil {
		if err == sql.ErrNoRows {
			// No sharelink found
			log.Printf("Folder %s has no sharelink", folderID)
			return false, nil, nil
		}
		return false, nil, fmt.Errorf("failed to check sharelink existence: %w", err)
	}

	log.Printf("Folder %s has sharelink with token: %s", folderID, token)
	return true, &token, nil
}

// CreateShareLink creates a new share link for a folder
func (fs *FolderService) CreateShareLink(folderID uuid.UUID, userID uuid.UUID) (*models.ShareLink, error) {
	// Check if user owns the folder
	_, err := fs.GetFolderByID(userID, folderID)
	if err != nil {
		return nil, err
	}

	// Check if a sharelink already exists for this folder
	existingShareLink, err := fs.GetShareLinkByFolderID(folderID)
	if err == nil && existingShareLink != nil {
		// Sharelink already exists, return it
		return existingShareLink, nil
	}

	// Create new sharelink for folder
	shareLink, err := models.NewFolderShareLink(folderID)
	if err != nil {
		return nil, fmt.Errorf("failed to create share link: %w", err)
	}

	// Insert into database
	query := `
		INSERT INTO sharelinks (id, file_id, folder_id, token, expires_at, is_active, download_count, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	
	_, err = fs.db.Exec(query, shareLink.ID, shareLink.FileID, shareLink.FolderID, shareLink.Token, shareLink.ExpiresAt, shareLink.IsActive, shareLink.DownloadCount, shareLink.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create share link in database: %w", err)
	}

	return shareLink, nil
}

// GetShareLinkByFolderID retrieves a share link for a folder
func (fs *FolderService) GetShareLinkByFolderID(folderID uuid.UUID) (*models.ShareLink, error) {
	log.Printf("[FOLDER-SERVICE] GetShareLinkByFolderID called - folderID: %s", folderID)
	
	// Use hardcoded query to avoid prepared statement issues
	query := fmt.Sprintf(`
		SELECT sharelink_f.id, sharelink_f.folder_id, sharelink_f.token, sharelink_f.expires_at, sharelink_f.is_active, sharelink_f.download_count, sharelink_f.created_at
		FROM sharelinks sharelink_f
		WHERE sharelink_f.folder_id = '%s'
	`, folderID.String())
	
	log.Printf("[FOLDER-SERVICE] GetShareLinkByFolderID query: %s", query)

	var shareLink models.ShareLink
	shareLink.FileID = nil // Explicitly set to nil for folder share links
	err := fs.db.QueryRow(query).Scan(
		&shareLink.ID,
		&shareLink.FolderID,
		&shareLink.Token,
		&shareLink.ExpiresAt,
		&shareLink.IsActive,
		&shareLink.DownloadCount,
		&shareLink.CreatedAt,
	)

	if err == sql.ErrNoRows {
		log.Printf("[FOLDER-SERVICE] GetShareLinkByFolderID: no share link found for folder %s", folderID)
		return nil, ErrShareLinkNotFound
	}
	if err != nil {
		log.Printf("[FOLDER-SERVICE] GetShareLinkByFolderID query failed: %v", err)
		return nil, fmt.Errorf("failed to get share link: %w", err)
	}

	log.Printf("[FOLDER-SERVICE] GetShareLinkByFolderID success - token: %s", shareLink.Token)
	return &shareLink, nil
}

// GetFolderByShareToken retrieves a folder by its share link token
func (fs *FolderService) GetFolderByShareToken(token string) (*models.Folder, error) {
	// First validate token format
	if !models.IsValidShareToken(token) {
		return nil, ErrInvalidShareToken
	}

	// Get sharelink by token
	query := `
		SELECT sl.id, sl.folder_id, sl.token, sl.expires_at, sl.is_active, sl.download_count, sl.created_at,
			   f.id, f.owner_id, f.name, f.parent_id, f.created_at, f.updated_at
		FROM sharelinks sl
		JOIN folders f ON sl.folder_id = f.id
		WHERE sl.token = $1 AND sl.is_active = true
	`

	var shareLink models.ShareLink
	var folder models.Folder
	shareLink.FileID = nil // Explicitly set to nil for folder share links
	err := fs.db.QueryRow(query, token).Scan(
		&shareLink.ID,
		&shareLink.FolderID,
		&shareLink.Token,
		&shareLink.ExpiresAt,
		&shareLink.IsActive,
		&shareLink.DownloadCount,
		&shareLink.CreatedAt,
		&folder.ID,
		&folder.OwnerID,
		&folder.Name,
		&folder.ParentID,
		&folder.CreatedAt,
		&folder.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrShareLinkNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get folder by share token: %w", err)
	}

	// Check if the sharelink has expired
	if shareLink.IsExpired() {
		return nil, ErrShareLinkExpired
	}

	return &folder, nil
}

// deleteFilesInFolderTree deletes all files within a folder tree using the file service
// This ensures proper blob cleanup and reference counting
func (fs *FolderService) deleteFilesInFolderTree(ownerID, folderID uuid.UUID) error {
	log.Printf("[FOLDER-SERVICE] deleteFilesInFolderTree called - ownerID: %s, folderID: %s", ownerID, folderID)
	
	// Get all files in the folder tree using a recursive CTE
	query := `
		WITH RECURSIVE folder_tree AS (
			-- Base case: the folder to delete
			SELECT id FROM folders WHERE id = $1 AND owner_id = $2
			UNION ALL
			-- Recursive case: child folders
			SELECT f.id FROM folders f
			INNER JOIN folder_tree ft ON f.parent_id = ft.id
		)
		SELECT files.id FROM files
		WHERE files.owner_id = $2 AND files.folder_id IN (SELECT id FROM folder_tree)
		ORDER BY files.id
	`

	rows, err := fs.db.Query(query, folderID, ownerID)
	if err != nil {
		return fmt.Errorf("failed to query files in folder tree: %w", err)
	}
	defer rows.Close()

	var fileIDs []uuid.UUID
	for rows.Next() {
		var fileID uuid.UUID
		err := rows.Scan(&fileID)
		if err != nil {
			return fmt.Errorf("failed to scan file ID: %w", err)
		}
		fileIDs = append(fileIDs, fileID)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error reading file IDs: %w", err)
	}

	log.Printf("[FOLDER-SERVICE] Found %d files to delete in folder tree", len(fileIDs))

	// Delete each file using the file service to ensure proper blob cleanup
	for _, fileID := range fileIDs {
		log.Printf("[FOLDER-SERVICE] Deleting file: %s", fileID)
		err := fs.fileService.DeleteFile(fileID, ownerID)
		if err != nil {
			log.Printf("[FOLDER-SERVICE] Warning: failed to delete file %s: %v", fileID, err)
			// Continue with other files even if one fails
		}
	}

	log.Printf("[FOLDER-SERVICE] Completed deleting files in folder tree")
	return nil
}

// GetAllFolders returns all folders owned by a user (flat list for client-side tree building)
func (fs *FolderService) GetAllFolders(ownerID uuid.UUID) ([]*models.Folder, error) {
	log.Printf("[FOLDER-SERVICE] GetAllFolders called - ownerID: %s", ownerID)
	
	query := `
		SELECT id, owner_id, name, parent_id, created_at, updated_at
		FROM folders 
		WHERE owner_id = $1
		ORDER BY parent_id NULLS FIRST, name ASC
	`

	rows, err := fs.db.Query(query, ownerID)
	if err != nil {
		log.Printf("[FOLDER-SERVICE] GetAllFolders query failed: %v", err)
		return nil, fmt.Errorf("failed to get all folders: %w", err)
	}
	defer rows.Close()

	var folders []*models.Folder
	for rows.Next() {
		folder := &models.Folder{}
		err := rows.Scan(
			&folder.ID, &folder.OwnerID, &folder.Name,
			&folder.ParentID, &folder.CreatedAt, &folder.UpdatedAt,
		)
		if err != nil {
			log.Printf("[FOLDER-SERVICE] Failed to scan folder: %v", err)
			return nil, fmt.Errorf("failed to scan folder: %w", err)
		}
		folders = append(folders, folder)
	}

	if err := rows.Err(); err != nil {
		log.Printf("[FOLDER-SERVICE] Error reading folders: %v", err)
		return nil, fmt.Errorf("error reading folders: %w", err)
	}
	
	log.Printf("[FOLDER-SERVICE] GetAllFolders returning %d folders", len(folders))
	return folders, nil
}

// FolderTreeItem represents a folder with its files and subfolders
type FolderTreeItem struct {
	Folder     *models.Folder     `json:"folder"`
	Files      []*models.File     `json:"files"`
	Subfolders []*FolderTreeItem  `json:"subfolders"`
}

// GetFolderTreeByID gets a complete folder tree starting from a specific folder
func (fs *FolderService) GetFolderTreeByID(ownerID, folderID uuid.UUID) (*FolderTreeItem, error) {
	log.Printf("[FOLDER-SERVICE] GetFolderTreeByID called - ownerID: %s, folderID: %s", ownerID, folderID)

	// First get the root folder
	rootFolder, err := fs.GetFolderByID(ownerID, folderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get root folder: %w", err)
	}

	// Get all folders in the subtree using recursive CTE
	folderQuery := `
		WITH RECURSIVE folder_tree AS (
			-- Base case: the root folder
			SELECT id, name, parent_id, owner_id, created_at, updated_at, 0 as depth
			FROM folders WHERE id = $1 AND owner_id = $2
			UNION ALL
			-- Recursive case: child folders
			SELECT f.id, f.name, f.parent_id, f.owner_id, f.created_at, f.updated_at, ft.depth + 1
			FROM folders f
			INNER JOIN folder_tree ft ON f.parent_id = ft.id
		)
		SELECT id, name, parent_id, owner_id, created_at, updated_at
		FROM folder_tree
		ORDER BY depth, name`

	folderRows, err := fs.db.Query(folderQuery, folderID, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get folder tree: %w", err)
	}
	defer folderRows.Close()

	// Map to store all folders by ID
	folderMap := make(map[uuid.UUID]*models.Folder)
	var allFolders []*models.Folder

	for folderRows.Next() {
		var folder models.Folder
		err := folderRows.Scan(
			&folder.ID, &folder.Name, &folder.ParentID, &folder.OwnerID,
			&folder.CreatedAt, &folder.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan folder: %w", err)
		}
		folderMap[folder.ID] = &folder
		allFolders = append(allFolders, &folder)
	}

	// Get all files in the folder tree
	fileQuery := `
		WITH RECURSIVE folder_tree AS (
			-- Base case: the root folder
			SELECT id FROM folders WHERE id = $1 AND owner_id = $2
			UNION ALL
			-- Recursive case: child folders
			SELECT f.id FROM folders f
			INNER JOIN folder_tree ft ON f.parent_id = ft.id
		)
		SELECT files.id, files.owner_id, files.blob_hash, files.original_filename, 
		       files.mime_type, files.size_bytes, files.is_public, files.download_count, 
		       files.tags, files.folder_id, files.created_at, files.updated_at
		FROM files
		WHERE files.folder_id IN (SELECT id FROM folder_tree)
		ORDER BY files.folder_id, files.original_filename`

	fileRows, err := fs.db.Query(fileQuery, folderID, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get files in folder tree: %w", err)
	}
	defer fileRows.Close()

	// Map to store files by folder ID
	filesByFolder := make(map[uuid.UUID][]*models.File)

	for fileRows.Next() {
		var file models.File
		err := fileRows.Scan(
			&file.ID, &file.OwnerID, &file.BlobHash, &file.OriginalFilename,
			&file.MimeType, &file.SizeBytes, &file.IsPublic, &file.DownloadCount,
			pq.Array(&file.Tags), &file.FolderID, &file.CreatedAt, &file.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan file: %w", err)
		}

		if file.FolderID != nil {
			filesByFolder[*file.FolderID] = append(filesByFolder[*file.FolderID], &file)
		}
	}

	// Build the tree structure recursively
	return fs.buildFolderTree(rootFolder, folderMap, filesByFolder), nil
}

// GetFolderTreeByShareToken gets a complete folder tree using a share token
func (fs *FolderService) GetFolderTreeByShareToken(token string) (*FolderTreeItem, error) {
	log.Printf("[FOLDER-SERVICE] GetFolderTreeByShareToken called - token: %s", token)

	// First get the folder using the share token
	folder, err := fs.GetFolderByShareToken(token)
	if err != nil {
		return nil, fmt.Errorf("failed to get folder by share token: %w", err)
	}

	// Now get the complete tree for this folder
	return fs.GetFolderTreeByID(folder.OwnerID, folder.ID)
}

// buildFolderTree recursively builds the folder tree structure
func (fs *FolderService) buildFolderTree(folder *models.Folder, folderMap map[uuid.UUID]*models.Folder, filesByFolder map[uuid.UUID][]*models.File) *FolderTreeItem {
	item := &FolderTreeItem{
		Folder:     folder,
		Files:      filesByFolder[folder.ID],
		Subfolders: make([]*FolderTreeItem, 0),
	}

	// Find and add subfolders
	for _, candidateFolder := range folderMap {
		if candidateFolder.ParentID != nil && *candidateFolder.ParentID == folder.ID {
			subItem := fs.buildFolderTree(candidateFolder, folderMap, filesByFolder)
			item.Subfolders = append(item.Subfolders, subItem)
		}
	}

	return item
}

// getAllFilesInFolderRecursive gets all file IDs under a folder and its subfolders recursively
func (fs *FolderService) getAllFilesInFolderRecursive(folderID uuid.UUID, userID uuid.UUID) ([]uuid.UUID, error) {
	// Verify user has access to the folder
	_, err := fs.GetFolderByID(userID, folderID)
	if err != nil {
		return nil, err
	}

	// Use recursive CTE to get all files in folder and subfolders
	query := `
		WITH RECURSIVE folder_tree AS (
			-- Base case: the target folder
			SELECT id, parent_id
			FROM folders 
			WHERE id = $1 AND owner_id = $2
			
			UNION ALL
			
			-- Recursive case: all subfolders
			SELECT f.id, f.parent_id
			FROM folders f
			INNER JOIN folder_tree ft ON f.parent_id = ft.id
			WHERE f.owner_id = $2
		)
		SELECT DISTINCT f.id
		FROM files f
		WHERE f.folder_id IN (SELECT id FROM folder_tree)
		  AND f.owner_id = $2
		ORDER BY f.id
	`

	rows, err := fs.db.Query(query, folderID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get files in folder recursively: %w", err)
	}
	defer rows.Close()

	var fileIDs []uuid.UUID
	for rows.Next() {
		var fileID uuid.UUID
		if err := rows.Scan(&fileID); err != nil {
			return nil, fmt.Errorf("failed to scan file ID: %w", err)
		}
		fileIDs = append(fileIDs, fileID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating file rows: %w", err)
	}

	return fileIDs, nil
}

// MakeAllFilesInFolderPublic makes all files in a folder and its subfolders public
// and tracks which files were originally public vs made public due to folder sharing
func (fs *FolderService) MakeAllFilesInFolderPublic(folderID uuid.UUID, userID uuid.UUID) error {
	log.Printf("Starting to make all files public for folder %s, user %s", folderID, userID)
	
	// Get all file IDs in the folder recursively
	fileIDs, err := fs.getAllFilesInFolderRecursive(folderID, userID)
	if err != nil {
		log.Printf("Failed to get files in folder %s: %v", folderID, err)
		return fmt.Errorf("failed to get files in folder: %w", err)
	}

	log.Printf("Found %d files in folder %s and its subfolders", len(fileIDs), folderID)

	if len(fileIDs) == 0 {
		log.Printf("No files found in folder %s, skipping publicity update", folderID)
		return nil
	}

	// Start transaction for atomicity
	tx, err := fs.db.Begin()
	if err != nil {
		log.Printf("Failed to begin transaction for folder %s: %v", folderID, err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var trackedFiles int
	var madePublicFiles int

	// For each file, track its original publicity state and make it public
	for _, fileID := range fileIDs {
		// Check if file is already public
		var isCurrentlyPublic bool
		checkQuery := `SELECT is_public FROM files WHERE id = $1 AND owner_id = $2`
		err := tx.QueryRow(checkQuery, fileID, userID).Scan(&isCurrentlyPublic)
		if err != nil {
			log.Printf("Failed to check publicity state for file %s: %v", fileID, err)
			return fmt.Errorf("failed to check file publicity state for file %s: %w", fileID, err)
		}

		log.Printf("File %s is currently public: %t", fileID, isCurrentlyPublic)

		// Insert tracking record (ignore if already exists for this folder-file combination)
		trackingQuery := `
			INSERT INTO folder_file_publicity_tracking (folder_id, file_id, was_originally_public)
			VALUES ($1, $2, $3)
			ON CONFLICT (folder_id, file_id) DO NOTHING
		`
		result, err := tx.Exec(trackingQuery, folderID, fileID, isCurrentlyPublic)
		if err != nil {
			log.Printf("Failed to insert tracking record for file %s: %v", fileID, err)
			return fmt.Errorf("failed to insert publicity tracking for file %s: %w", fileID, err)
		}

		rowsAffected, err := result.RowsAffected()
		if err == nil && rowsAffected > 0 {
			trackedFiles++
			log.Printf("Tracked file %s (originally public: %t)", fileID, isCurrentlyPublic)
		} else {
			log.Printf("File %s was already tracked for folder %s", fileID, folderID)
		}

		// Make file public if it's not already
		if !isCurrentlyPublic {
			updateQuery := `UPDATE files SET is_public = true WHERE id = $1 AND owner_id = $2`
			result, err = tx.Exec(updateQuery, fileID, userID)
			if err != nil {
				log.Printf("Failed to make file %s public: %v", fileID, err)
				return fmt.Errorf("failed to make file %s public: %w", fileID, err)
			}

			rowsAffected, err = result.RowsAffected()
			if err == nil && rowsAffected > 0 {
				madePublicFiles++
				log.Printf("Made file %s public due to folder %s sharing", fileID, folderID)
			}
		} else {
			log.Printf("File %s was already public, no update needed", fileID)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		log.Printf("Failed to commit transaction for folder %s: %v", folderID, err)
		return fmt.Errorf("failed to commit file publicity changes: %w", err)
	}

	log.Printf("Successfully tracked %d files and made %d files public for folder %s", trackedFiles, madePublicFiles, folderID)
	return nil
}

// RevertFilesInFolderToOriginalState reverts files to their original publicity state
// when a folder sharelink is deleted, based on the tracking data
func (fs *FolderService) RevertFilesInFolderToOriginalState(folderID uuid.UUID, userID uuid.UUID) error {
	log.Printf("Starting file publicity reversion for folder %s, user %s", folderID, userID)
	
	// Verify user has access to the folder
	_, err := fs.GetFolderByID(userID, folderID)
	if err != nil {
		log.Printf("Failed to get folder %s for user %s: %v", folderID, userID, err)
		return err
	}

	// Get all tracked files for this folder first (without transaction)
	trackingQuery := `
		SELECT file_id, was_originally_public
		FROM folder_file_publicity_tracking
		WHERE folder_id = $1
	`
	log.Printf("Querying tracking data for folder %s", folderID)
	rows, err := fs.db.Query(trackingQuery, folderID)
	if err != nil {
		log.Printf("Failed to query tracking data for folder %s: %v", folderID, err)
		return fmt.Errorf("failed to get publicity tracking data: %w", err)
	}
	defer rows.Close()

	// Collect all tracking data first
	type trackingEntry struct {
		fileID              uuid.UUID
		wasOriginallyPublic bool
	}
	var trackingEntries []trackingEntry

	for rows.Next() {
		var entry trackingEntry
		if err := rows.Scan(&entry.fileID, &entry.wasOriginallyPublic); err != nil {
			log.Printf("Failed to scan tracking data: %v", err)
			return fmt.Errorf("failed to scan tracking data: %w", err)
		}
		trackingEntries = append(trackingEntries, entry)
	}

	if err := rows.Err(); err != nil {
		log.Printf("Error iterating tracking rows: %v", err)
		return fmt.Errorf("error iterating tracking rows: %w", err)
	}

	log.Printf("Found %d tracking entries for folder %s", len(trackingEntries), folderID)

	if len(trackingEntries) == 0 {
		log.Printf("No tracking entries found for folder %s - may have already been cleaned up", folderID)
		return nil
	}

	// Now start transaction for updates
	tx, err := fs.db.Begin()
	if err != nil {
		log.Printf("Failed to begin transaction for folder %s: %v", folderID, err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var revertedCount int
	var keptPublicCount int
	
	// Process each file
	for _, entry := range trackingEntries {
		log.Printf("Processing file %s (originally public: %t)", entry.fileID, entry.wasOriginallyPublic)

		// Revert ALL files to their original state based on tracking data
		updateQuery := `UPDATE files SET is_public = $1 WHERE id = $2 AND owner_id = $3`
		result, err := tx.Exec(updateQuery, entry.wasOriginallyPublic, entry.fileID, userID)
		if err != nil {
			log.Printf("Failed to update file %s: %v", entry.fileID, err)
			return fmt.Errorf("failed to revert file %s to original state: %w", entry.fileID, err)
		}

		rowsAffected, err := result.RowsAffected()
		if err == nil && rowsAffected > 0 {
			if entry.wasOriginallyPublic {
				keptPublicCount++
				log.Printf("Kept file %s public (was originally public)", entry.fileID)
			} else {
				revertedCount++
				log.Printf("Reverted file %s to private (was originally private)", entry.fileID)
			}
		} else {
			log.Printf("Warning: No rows affected for file %s (may not exist or not owned by user)", entry.fileID)
		}
	}

	// Clean up tracking data for this folder
	log.Printf("Cleaning up tracking data for folder %s", folderID)
	deleteTrackingQuery := `DELETE FROM folder_file_publicity_tracking WHERE folder_id = $1`
	result, err := tx.Exec(deleteTrackingQuery, folderID)
	if err != nil {
		log.Printf("Failed to clean up tracking data for folder %s: %v", folderID, err)
		return fmt.Errorf("failed to clean up publicity tracking data: %w", err)
	}

	deletedRows, err := result.RowsAffected()
	if err == nil {
		log.Printf("Deleted %d tracking entries for folder %s", deletedRows, folderID)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		log.Printf("Failed to commit transaction for folder %s: %v", folderID, err)
		return fmt.Errorf("failed to commit file publicity reversion: %w", err)
	}

	log.Printf("Successfully reverted %d files to private and kept %d files public for folder %s", revertedCount, keptPublicCount, folderID)
	return nil
}