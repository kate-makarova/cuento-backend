package Router

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

type RouteDefinition struct {
	Method     string
	Path       string
	Definition string
}

type CustomRouter struct {
	router      *gin.RouterGroup
	definitions []RouteDefinition
}

var AllRoutes []RouteDefinition

func NewCustomRouter(router *gin.RouterGroup) *CustomRouter {
	return &CustomRouter{
		router:      router,
		definitions: []RouteDefinition{},
	}
}

func (cr *CustomRouter) GET(relativePath string, definition string, handlers ...gin.HandlerFunc) gin.IRoutes {
	route := RouteDefinition{
		Method:     "GET",
		Path:       relativePath,
		Definition: definition,
	}
	cr.definitions = append(cr.definitions, route)
	AllRoutes = append(AllRoutes, route)
	return cr.router.GET(relativePath, handlers...)
}

func (cr *CustomRouter) POST(relativePath string, definition string, handlers ...gin.HandlerFunc) gin.IRoutes {
	route := RouteDefinition{
		Method:     "POST",
		Path:       relativePath,
		Definition: definition,
	}
	cr.definitions = append(cr.definitions, route)
	AllRoutes = append(AllRoutes, route)
	return cr.router.POST(relativePath, handlers...)
}

func (cr *CustomRouter) PATCH(relativePath string, definition string, handlers ...gin.HandlerFunc) gin.IRoutes {
	route := RouteDefinition{
		Method:     "PATCH",
		Path:       relativePath,
		Definition: definition,
	}
	cr.definitions = append(cr.definitions, route)
	AllRoutes = append(AllRoutes, route)
	return cr.router.PATCH(relativePath, handlers...)
}

func (cr *CustomRouter) PUT(relativePath string, definition string, handlers ...gin.HandlerFunc) gin.IRoutes {
	route := RouteDefinition{
		Method:     "PUT",
		Path:       relativePath,
		Definition: definition,
	}
	cr.definitions = append(cr.definitions, route)
	AllRoutes = append(AllRoutes, route)
	return cr.router.PUT(relativePath, handlers...)
}

func (cr *CustomRouter) DELETE(relativePath string, definition string, handlers ...gin.HandlerFunc) gin.IRoutes {
	route := RouteDefinition{
		Method:     "DELETE",
		Path:       relativePath,
		Definition: definition,
	}
	cr.definitions = append(cr.definitions, route)
	AllRoutes = append(AllRoutes, route)
	return cr.router.DELETE(relativePath, handlers...)
}

func (cr *CustomRouter) PrintRoutes() {
	for _, def := range cr.definitions {
		fmt.Printf("%s - %s - %s\n", def.Path, def.Method, def.Definition)
	}
}

func (cr *CustomRouter) GetRoutes() []RouteDefinition {
	return cr.definitions
}
