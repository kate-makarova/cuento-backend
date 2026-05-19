package Controllers

import (
	"cuento-backend/src/Middlewares"
	"cuento-backend/src/Services"
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

const frontendDir = "./../frontend"

type frontendComponent struct {
	Name                string `json:"name"`
	TemplatePath        string `json:"template_path"`
	DefaultTemplatePath string `json:"default_template_path"`
	Description         string `json:"description"`
	Active              bool   `json:"active"`
}

type frontendComponentDef struct {
	Name                string
	TemplatePath        string
	DefaultTemplatePath string
	DescriptionKey      string
}

var frontendComponentDefs = []frontendComponentDef{
	{
		Name:                "src/app/components/header",
		TemplatePath:        "src/app/components/header/header.custom.component.html",
		DefaultTemplatePath: "src/app/components/header/header.component.html",
		DescriptionKey:      "frontend_component.src_app_components_header.description",
	},
}

func readActiveCustomTemplates() map[string]bool {
	envPath := filepath.Join(frontendDir, "src/environments/environment.prod.ts")
	data, err := os.ReadFile(envPath)
	if err != nil {
		return map[string]bool{}
	}
	active := map[string]bool{}
	for _, match := range customTemplateRe.FindAllSubmatch(data, -1) {
		active[string(match[1])] = true
	}
	return active
}

func GetFrontendComponents(c *gin.Context, db *sql.DB) {
	userID := Services.GetUserIdFromContext(c)
	lang := Services.GetUserLanguage(userID, db)
	localizer := Services.NewLocalizer(lang)
	active := readActiveCustomTemplates()

	result := make([]frontendComponent, len(frontendComponentDefs))
	for i, def := range frontendComponentDefs {
		result[i] = frontendComponent{
			Name:                def.Name,
			TemplatePath:        def.TemplatePath,
			DefaultTemplatePath: def.DefaultTemplatePath,
			Description:         Services.T(localizer, def.DescriptionKey),
			Active:              active[def.DefaultTemplatePath],
		}
	}
	c.JSON(http.StatusOK, result)
}

func readComponentFile(c *gin.Context, relPath string) {
	absPath := filepath.Join(frontendDir, relPath)
	content, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusNotFound, Message: "Template file not found: " + relPath})
		} else {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to read template file: " + err.Error()})
		}
		c.Abort()
		return
	}
	c.String(http.StatusOK, string(content))
}

func findComponentDef(name string) (frontendComponentDef, bool) {
	for _, def := range frontendComponentDefs {
		if def.Name == name {
			return def, true
		}
	}
	return frontendComponentDef{}, false
}

func GetFrontendComponentTemplate(c *gin.Context) {
	name := c.Param("name")
	def, ok := findComponentDef(name)
	if !ok {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusNotFound, Message: "Unknown component: " + name})
		c.Abort()
		return
	}
	readComponentFile(c, def.TemplatePath)
}

func GetFrontendComponentDefaultTemplate(c *gin.Context) {
	name := c.Param("name")
	def, ok := findComponentDef(name)
	if !ok {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusNotFound, Message: "Unknown component: " + name})
		c.Abort()
		return
	}
	readComponentFile(c, def.DefaultTemplatePath)
}

var customTemplateRe = regexp.MustCompile(`component:\s*['"]([^'"]+)['"]\s*,\s*template:\s*['"]([^'"]+)['"]`)
var customTemplatesBlockRe = regexp.MustCompile(`(?s)customTemplates:\s*\[.*?\]\s*as\s*\{[^}]+\}\[\]`)

type updateEnvRequest struct {
	ActiveComponents []string `json:"active_components"`
}

func UpdateFrontendEnv(c *gin.Context, db *sql.DB) {
	var req updateEnvRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid request body: " + err.Error()})
		c.Abort()
		return
	}

	envPath := filepath.Join(frontendDir, "src/environments/environment.prod.ts")
	data, err := os.ReadFile(envPath)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to read environment file: " + err.Error()})
		c.Abort()
		return
	}

	activeSet := make(map[string]bool, len(req.ActiveComponents))
	for _, name := range req.ActiveComponents {
		activeSet[name] = true
	}

	var entries []string
	for _, def := range frontendComponentDefs {
		if activeSet[def.Name] {
			entries = append(entries, "    { component: '"+def.DefaultTemplatePath+"', template: '"+def.TemplatePath+"' }")
		}
	}

	var newBlock string
	if len(entries) == 0 {
		newBlock = "customTemplates: [] as { component: string; template: string }[]"
	} else {
		newBlock = "customTemplates: [\n" + strings.Join(entries, ",\n") + "\n  ] as { component: string; template: string }[]"
	}

	updated := customTemplatesBlockRe.ReplaceAll(data, []byte(newBlock))
	if string(updated) == string(data) {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Could not locate customTemplates block in environment file"})
		c.Abort()
		return
	}

	cfg, err := Services.GetGitHubConfig(db)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "GitHub config error: " + err.Error()})
		c.Abort()
		return
	}

	files := []Services.GitHubFile{{Path: "src/environments/environment.prod.ts", Content: string(updated)}}
	if err := Services.GitHubCommit(cfg, "Update custom templates configuration", files); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "GitHub commit failed: " + err.Error()})
		c.Abort()
		return
	}

	c.JSON(http.StatusOK, gin.H{"active_components": req.ActiveComponents})
}

type updateComponentTemplateRequest struct {
	Name    string `json:"name"    binding:"required"`
	Content string `json:"content" binding:"required"`
}

func UpdateFrontendComponentTemplate(c *gin.Context, db *sql.DB) {
	var req updateComponentTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid request body: " + err.Error()})
		c.Abort()
		return
	}

	def, ok := findComponentDef(req.Name)
	if !ok {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusNotFound, Message: "Unknown component: " + req.Name})
		c.Abort()
		return
	}

	cfg, err := Services.GetGitHubConfig(db)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "GitHub config error: " + err.Error()})
		c.Abort()
		return
	}

	files := []Services.GitHubFile{{Path: def.TemplatePath, Content: req.Content}}
	if err := Services.GitHubCommit(cfg, "Update "+def.Name+" template", files); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "GitHub commit failed: " + err.Error()})
		c.Abort()
		return
	}

	c.JSON(http.StatusOK, gin.H{"committed": def.TemplatePath})
}

type commitRequest struct {
	Message string                `json:"message" binding:"required"`
	Files   []Services.GitHubFile `json:"files"   binding:"required,min=1"`
}

func CommitFrontendTemplates(c *gin.Context, db *sql.DB) {
	var req commitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid request body: " + err.Error()})
		c.Abort()
		return
	}

	cfg, err := Services.GetGitHubConfig(db)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "GitHub config error: " + err.Error()})
		c.Abort()
		return
	}

	if err := Services.GitHubCommit(cfg, req.Message, req.Files); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "GitHub commit failed: " + err.Error()})
		c.Abort()
		return
	}

	c.JSON(http.StatusOK, gin.H{"committed": len(req.Files)})
}
