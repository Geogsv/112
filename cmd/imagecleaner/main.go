package main

import (
	"imagecleaner/internal/database" // Импортируем пакет для работы с базой данных
	"imagecleaner/internal/handlers" // Импортируем пакет для работы с обработчиками запросов
	"imagecleaner/internal/middleware"
	"log"
	"net/http"
	"os"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	log.Printf("Переменная окружения %s не установлена, используется значение по умолчанию: %s", key, fallback)
	return fallback
}



// func checkAndCreateUploadsDir(uploadDir string) {
// 	if _, err := os.Stat(uploadDir); os.IsNotExist(err) {
// 		log.Printf("Папка %s не найдена, создаем...", uploadDir)
// 		// Используем MkdirAll на случай, если и родительских папок нет
// 		if err := os.MkdirAll(uploadDir, 0755); err != nil {
// 			log.Fatalf("КРИТИЧЕСКАЯ ОШИБКА: не удалось создать папку %s: %v", uploadDir, err)
// 		}
// 		log.Printf("Папка %s успешно создана.", uploadDir)
// 	} else if err != nil {
// 		log.Fatalf("КРИТИЧЕСКАЯ ОШИБКА: ошибка при проверке папки %s: %v", uploadDir, err)
// 	} else {
// 		log.Printf("Папка %s найдена.", uploadDir)
// 	}
// }
func main() {
	cookieSecret := getEnv("COOKIE_SECRET", "fallback-secret-change-in-production")
	dbPath := getEnv("DB_PATH", "/app/service.db") // Путь ВНУТРИ контейнера
	listenPort := getEnv("LISTEN_PORT", "8080")

	// Инициализация базы данных
	err := database.InitDB(dbPath)
	if err != nil {
		log.Fatalf("Ошибка инициализации базы данных: %v", err)
	}

	gin.SetMode(gin.ReleaseMode)
	// Создаем роутер с настройками по умолчанию
	router := gin.Default()

	err = router.SetTrustedProxies([]string{"127.0.0.1", "::1"})
	if err != nil {
     	log.Fatalf("Ошибка установки доверенных прокси: %v", err)
	}
	router.MaxMultipartMemory = 10 << 20 // 8 MiB
	// Создаем новое хранилище куки с секретным ключом
	store := cookie.NewStore([]byte(cookieSecret))
	// Настраиваем параметры куку сессий
	store.Options(sessions.Options{
		Path:     "/", // Cookie доступен для всего сайта
		MaxAge:   86400 * 7,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
	})

	//
	router.Use(sessions.Sessions("mysession", store))


	// Загрузка шаблонов HTML из директории templates
	//Указываем gin где иска файлы шаблонов
	router.LoadHTMLGlob("web/templates/*")
	router.Static("/static", "./web/static")


	// Определяем маршрут (Route)
	router.GET("/", handlers.ShowLoginPage)
	router.GET("/login", handlers.ShowLoginPage)
	router.POST("/login", handlers.HandleLogin)
	router.GET("/register", handlers.ShowRegisterPage)
	router.POST("/register", handlers.HandleRegister)	

	// Маршруты для просмотра изображений
	router.GET("/view/:token", handlers.ShowConfirmViewPage)  // Показ страницы подтверждения
	router.POST("/view/:token", handlers.HandleConfirmView) // Обработка подтверждения и отдача файла

	// Группа маршрутов, требующих аутентификации
    protected := router.Group("/")
    protected.Use(middleware.AuthRequired())
    {
        protected.GET("/upload", handlers.ShowUploadPage)
        protected.POST("/upload", handlers.HandleUpload)
        protected.POST("/logout", handlers.HandleLogout)
    }
	// Конец ЗГРУППЫ
	listenAddr := ":" + listenPort
	// ЛОгирование работы сервера
	log.Printf("Сервер запускаеться на порту :%s", listenAddr)

	// Запускаем сервер на порту 8080
	// если порт занят или возникла другая ошибка, то программа завершится с ошибкой
	err = router.Run(listenAddr)
	if err != nil {
		log.Fatalf("Не удалось запустить сервер: %v", err)
	}
}
