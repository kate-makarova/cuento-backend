package Controllers

import (
	"cuento-backend/src/Middlewares"
	"cuento-backend/src/Services"
	"database/sql"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

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

var customTemplateRe = regexp.MustCompile(`component:\s*['"]([^'"]+)['"]\s*,\s*template:\s*['"]([^'"]+)['"]`)
var customTemplatesBlockRe = regexp.MustCompile(`(?s)customTemplates:\s*\[.*?]\s*as\s*\{[^}]+}\[]`)

func findComponentDef(name string) (frontendComponentDef, bool) {
	for _, def := range frontendComponentDefs {
		if def.Name == name {
			return def, true
		}
	}
	return frontendComponentDef{}, false
}

func readActiveCustomTemplates(cfg Services.GitHubConfig) map[string]bool {
	data, err := Services.GitHubGetFile(cfg, "src/environments/environment.prod.ts")
	if err != nil {
		return map[string]bool{}
	}
	active := map[string]bool{}
	for _, match := range customTemplateRe.FindAllStringSubmatch(data, -1) {
		active[match[1]] = true
	}
	return active
}

func GetFrontendComponents(c *gin.Context, db *sql.DB) {
	cfg, err := Services.GetGitHubConfig(db)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "GitHub config error: " + err.Error()})
		c.Abort()
		return
	}

	userID := Services.GetUserIdFromContext(c)
	lang := Services.GetUserLanguage(userID, db)
	localizer := Services.NewLocalizer(lang)
	active := readActiveCustomTemplates(cfg)

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

func getComponentFile(c *gin.Context, db *sql.DB, path string) {
	cfg, err := Services.GetGitHubConfig(db)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "GitHub config error: " + err.Error()})
		c.Abort()
		return
	}
	content, err := Services.GitHubGetFile(cfg, path)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to fetch file from GitHub: " + err.Error()})
		c.Abort()
		return
	}
	c.String(http.StatusOK, content)
}

func GetFrontendComponentTemplate(c *gin.Context, db *sql.DB) {
	name := strings.TrimPrefix(c.Param("name"), "/")
	def, ok := findComponentDef(name)
	if !ok {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusNotFound, Message: "Unknown component: " + name})
		c.Abort()
		return
	}
	getComponentFile(c, db, def.TemplatePath)
}

func GetFrontendComponentDefaultTemplate(c *gin.Context, db *sql.DB) {
	name := strings.TrimPrefix(c.Param("name"), "/")
	def, ok := findComponentDef(name)
	if !ok {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusNotFound, Message: "Unknown component: " + name})
		c.Abort()
		return
	}
	getComponentFile(c, db, def.DefaultTemplatePath)
}

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

	cfg, err := Services.GetGitHubConfig(db)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "GitHub config error: " + err.Error()})
		c.Abort()
		return
	}

	const envFilePath = "src/environments/environment.prod.ts"
	current, err := Services.GitHubGetFile(cfg, envFilePath)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to fetch environment file: " + err.Error()})
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

	updated := customTemplatesBlockRe.ReplaceAllString(current, newBlock)
	if updated == current {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Could not locate customTemplates block in environment file"})
		c.Abort()
		return
	}

	files := []Services.GitHubFile{{Path: envFilePath, Content: updated}}
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
