package contact

import (
	"bytes"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/jinzhu/gorm"
	lib "github.com/maxiloEmmmm/go-tool"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type GinHelp struct {
	*gin.Context
}

type page struct {
	Current         int
	Size            int
	PageKey         string
	PageSizeKey     string
	PageSizeDefault int
}

var GinPage = page{Current: 1, Size: 10, PageKey: "page", PageSizeKey: "page_size", PageSizeDefault: 15}

func GinGormPageHelp(db *gorm.DB, data interface{}) int {
	return GinGormPageBase(db, data, GinPage.Current, GinPage.Size)
}

func GinGormPageHelpWithOptionSize(db *gorm.DB, data interface{}, size int) int {
	return GinGormPageBase(db, data, GinPage.Current, size)
}

func GinGormPageHelpWithOption(db *gorm.DB, data interface{}, current int, size int) int {
	return GinGormPageBase(db, data, current, size)
}

func GinGormPageBase(db *gorm.DB, data interface{}, current int, size int) (total int) {
	lib.AssetsError(db.Model(data).Count(&total).Error)
	lib.AssetsError(db.Offset((current - 1) * size).Limit(size).Find(data).Error)
	return
}

type GinHelpHandlerFunc func(c *GinHelp)

type H map[string]interface{}

func GinRouteAuth() gin.HandlerFunc {
	return GinHelpHandle(func(c *GinHelp) {
		token := c.GetToken()

		jwt := JwtNew()

		jwt.SetSecret(Config.Jwt.Secret)

		if err := jwt.ParseToken(token); err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, err.Error())
		} else {
			c.Set("auth", jwt)
			c.Next()
		}
	})
}

type Cors struct {
	AllowOrigin  []string
	AllowHeaders []string
	AllowMethods []string
	// response header without Cache-Control、Content-Language、Content-Type、Expires、Last-Modified、Pragma
	// if client will get...
	ExposeHeaders    []string
	AllowCredentials bool
}

var CorsConfig = Cors{
	AllowOrigin:      []string{"*"},
	AllowHeaders:     []string{"Content-Type", "AccessToken", "X-CSRF-Token", "Authorization", "X-Requested-With"},
	AllowMethods:     []string{"POST", "GET", "OPTIONS", "PUT", "PATCH", "DELETE"},
	AllowCredentials: false,
}

func GinCors() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method

		origin := c.GetHeader("origin")
		if lib.InArray(CorsConfig.AllowOrigin, "*") || lib.InArray(CorsConfig.AllowOrigin, origin) {
			if method == "OPTIONS" {
				c.AbortWithStatus(http.StatusNoContent)
			}
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Headers", strings.Join(CorsConfig.AllowHeaders, ","))
			c.Header("Access-Control-Allow-Methods", strings.Join(CorsConfig.AllowMethods, ","))
			c.Header("Access-Control-Expose-Headers", strings.Join(CorsConfig.ExposeHeaders, ","))
			c.Header("Access-Control-Allow-Credentials", lib.AssetsReturn(CorsConfig.AllowCredentials, "true", "false").(string))
		}
		c.Next()
	}
}

func InitGin() {
	switch Config.App.Mode {
	case "debug", "":
		gin.SetMode(gin.DebugMode)
	case "release":
		gin.SetMode(gin.ReleaseMode)
	case "test":
		gin.SetMode(gin.TestMode)
	default:
		gin.SetMode(gin.DebugMode)
	}
}

var ServerErrorWrite = new(ServerErrorIO)

type ServerErrorIO struct{}

func (sew ServerErrorIO) Write(p []byte) (n int, err error) {
	buffer := new(bytes.Buffer)
	buffer.Write([]byte("[SERVER_ERROR]:"))
	buffer.Write(p)
	return gin.DefaultWriter.Write(buffer.Bytes())
}

func GinHelpHandle(h GinHelpHandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if page, err := strconv.Atoi(c.DefaultQuery(GinPage.PageKey, "1")); err == nil {
			GinPage.Current = page
		}

		if pageSize, err := strconv.Atoi(c.DefaultQuery(GinPage.PageSizeKey, string(GinPage.PageSizeDefault))); err == nil {
			GinPage.Size = pageSize
		}

		help := &GinHelp{c}
		defer func(c *GinHelp) {
			if err := recover(); err != nil {
				switch err.(type) {
				case ResponseAbortError:
					{
						return
					}
				default:
					{
						errMsg := ""
						if e, ok := err.(error); ok {
							errMsg = e.(error).Error()
						} else {
							errMsg = fmt.Sprintf("%+v", err)
						}

						md5 := lib.Md5(fmt.Sprintf("%d%s", time.Now().Unix(), errMsg))
						ServerErrorWrite.Write([]byte(lib.StringJoin(md5, "-", errMsg)))
						c.AbortWithStatusJSON(http.StatusUnprocessableEntity, H{
							"code":    "server",
							"message": md5,
						})
					}
				}
			}
		}(help)
		h(help)
	}
}

type ResponseAbortError struct{}

func (r ResponseAbortError) Error() string {
	return "abort"
}

// 响应
func (help *GinHelp) Response(code int, jsonObj interface{}) {
	help.AbortWithStatusJSON(code, jsonObj)
	panic(ResponseAbortError{})
}

// 分页响应辅助
func (help *GinHelp) ResourcePage(data interface{}, total int) {
	help.Resource(H{
		"data":  data,
		"total": total,
	})
}

// 成功响应
func (help *GinHelp) Resource(data interface{}) {
	help.Response(http.StatusOK, data)
}

// 创建成功响应
func (help *GinHelp) ResourceCreate(data interface{}) {
	help.Response(http.StatusCreated, data)
}

// 删除成功响应
func (help *GinHelp) ResourceDelete() {
	help.Response(http.StatusNoContent, "")
}

// 资源丢失响应
func (help *GinHelp) ResourceNotFound() {
	help.Response(http.StatusNotFound, "")
}

// 客户端错误响应
func (help *GinHelp) InValid(code string, msg string) {
	help.Response(http.StatusUnprocessableEntity, H{
		"code":    code,
		"message": msg,
	})
}

// 断言客户端错误
func (help *GinHelp) AssetsInValid(code string, err error) {
	if err != nil {
		switch err.(type) {
		case validator.ValidationErrors:
			{
				if errors, ok := err.(validator.ValidationErrors); ok && len(errors) > 0 {
					e := errors[0]
					help.InValid(code, lib.StringJoin(
						e.Translate(Tranintance.Tran),
						", 类型: ", e.Type().Name(),
						", 当前值: ", fmt.Sprintf("%v", e.Value()),
						", 数据结构路径: ", e.StructNamespace()))
				} else {
					help.InValidError(code, err)
				}
			}
		default:
			help.InValidError(code, err)
		}
	}
}

// 客户端错误响应
func (help *GinHelp) InValidError(code string, err error) {
	help.InValid(code, err.Error())
}

// 客户端query错误响应
func (help *GinHelp) InValidBindQuery(query interface{}) {
	help.AssetsInValid("input:query", help.ShouldBindQuery(query))
}

// 客户端uri错误响应
func (help *GinHelp) InValidBindUri(query interface{}) {
	help.AssetsInValid("input:uri", help.ShouldBindUri(query))
}

// 客户端body错误响应
func (help *GinHelp) InValidBind(json interface{}) {
	help.AssetsInValid("input:body", help.ShouldBind(json))
}

// 客户端未认证 不推荐使用 太过于底层 推荐InValid*
func (help *GinHelp) Unauthorized(msg string) {
	help.Response(http.StatusUnauthorized, map[string]interface{}{"msg": msg})
}

// 客户端错误请求 不推荐使用 太过于底层 推荐InValid*
func (help *GinHelp) BadRequest(msg string) {
	help.Response(http.StatusBadRequest, map[string]interface{}{"msg": msg})
}

// 客户端权限不足 不推荐使用 太过于底层 推荐InValid*
func (help *GinHelp) Forbidden(msg string) {
	help.Response(http.StatusForbidden, map[string]interface{}{"msg": msg})
}

// 服务端错误响应 不推荐使用 太过于底层 推荐InValid*
func (help *GinHelp) ServerError(msg string) {
	help.Response(http.StatusInternalServerError, map[string]interface{}{"msg": msg})
}

// 获取token
func (help *GinHelp) GetToken() string {
	token := help.GetHeader("Authorization")

	if len(token) == 0 {
		token, _ = help.GetQuery("token")
	}

	return token
}
