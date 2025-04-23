package middleware

import (
	"net/http"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

func AuthRequired() gin.HandlerFunc {
	// Возвращаем функцию, которая соответствует типу gin.HandlerFunc
	return func(c *gin.Context) {
		// Получаем текущую сессию
		session := sessions.Default(c)
		// Пытаемся получить userID из сессии.
		// Мы сохраняли его как int64 при логине.
		userID := session.Get("userID")
		if userID == nil {
			// Пользователь не аутентифицирован!
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		
		// Пользователь аутентифицирован.
		if id, ok := userID.(int64); ok {
			c.Set("userID", id)
		}else{
			session.Delete("userID")
			session.Delete("username")
			session.Save()
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		c.Next()
	}
}