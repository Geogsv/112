package main

import (
	"imagecleaner/internal/database" // Импортируем пакет для работы с базой данных
	"imagecleaner/internal/handlers" // Импортируем пакет для работы с обработчиками запросов
	"imagecleaner/internal/middleware"
	"imagecleaner/internal/services"
	"log"
	"net/http"
	"os"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)
const dbFileName = "service.db"

// checkAndCreateUploadsDir проверяет наличие папки uploads и создает ее при необходимости.
func checkAndCreateUploadsDir() {
	uploadDir := services.UploadPath // Используем путь из services
	if _, err := os.Stat(uploadDir); os.IsNotExist(err) {
		log.Printf("Папка %s не найдена, создаем...", uploadDir)
		err = os.Mkdir(uploadDir, 0755) // 0755 - права доступа (владелец rwx, группа rx, остальные rx)
		if err != nil {
			// Если не удалось создать папку - это критично
			log.Fatalf("КРИТИЧЕСКАЯ ОШИБКА: не удалось создать папку %s: %v", uploadDir, err)
		}
		log.Printf("Папка %s успешно создана.", uploadDir)
	} else if err != nil {
		// Другая ошибка при проверке папки (например, нет прав доступа)
		log.Fatalf("КРИТИЧЕСКАЯ ОШИБКА: ошибка при проверке папки %s: %v", uploadDir, err)
	} else {
		log.Printf("Папка %s найдена.", uploadDir)
	}
}
func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Не удалось загрузить файл .env: %v", err)
	}
	checkAndCreateUploadsDir()

	// Инициализация базы данных
	err = database.InitDB(dbFileName)
	if err != nil {
		log.Fatalf("Ошибка инициализации базы данных: %v", err)
	}
	cookieSecret := os.Getenv("COOKIE_SECRET")
	if cookieSecret == "" {
		log.Fatal("Не удалось получить секретный ключ из переменной окружения COOKIE_SECRET")
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

	// ЛОгирование работы сервера
	log.Println("Сервер запускаеться на порту :8080")

	// Запускаем сервер на порту 8080
	// если порт занят или возникла другая ошибка, то программа завершится с ошибкой
	err = router.Run(":8080")
	if err != nil {
		log.Fatalf("Не удалось запустить сервер: %v", err)
	}
}
