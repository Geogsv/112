package handlers

import (
	"fmt"
	"imagecleaner/internal/auth"

	"imagecleaner/internal/database"
	"imagecleaner/internal/services"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

const MaxUploadSize = 10 << 20 // 10 МБ
const MaxFiles = 10 // Максимальное количество загружаемых файлов

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	// Лог не нужен здесь, есть в main
	return fallback
}

func ShowLoginPage(c *gin.Context) {
	// c.HTML рендерит шаблон.
	// http.StatusOK - код ответа 200 OK.
	// "login.html" - имя файла шаблона (Gin найдет его в web/templates/).
	// gin.H{...} - карта для передачи данных в шаблон.
	c.HTML(http.StatusOK, "login.html", gin.H{
		"title": "Вход", // Передаем заголовок страницы в шаблон.
	})
}


func ShowRegisterPage(c *gin.Context) {
	queryParams := c.Request.URL.Query()
	errorMsg := queryParams.Get("error")
	successMsg := queryParams.Get("success")
	usernameValue := queryParams.Get("username")
	c.HTML(http.StatusOK, "register.html", gin.H{
		"title":      "Регистрация",
		"error":      errorMsg,
		"success":    successMsg,
		"username":   usernameValue,
	})
}
// Обработчик регистрации
func HandleRegister(c *gin.Context) {
	// 1. Получаем данные из формы
	username := strings.TrimSpace(c.PostForm("username"))
	password := strings.TrimSpace(c.PostForm("password"))
	passwordConfirm := strings.TrimSpace(c.PostForm("password_confirm"))

	// Проверяем, что поля не пустые
	if username == "" || password == "" || passwordConfirm == "" {
		// Если данные неверны, снова показываем страницу регистрации,
		// передавая сообщение об ошибке и введенное имя пользователя (чтобы не вводить заново).
		c.HTML(http.StatusBadRequest, "register.html", gin.H{
			"title":       "Регистрация",
			"error":       "Все поля должны быть заполнены",
			"username":    username,
		})
		return // Выходим из функции, если данные не валидны.
	}
	// Проверяем, что пароль достаточно длинный
	if len(password) < 8 {
		// Если пароль короче 8 символов, снова показываем страницу регистрации,
		// передавая сообщение об ошибке и введенное имя пользователя (чтобы не вводить заново).
		c.HTML(http.StatusBadRequest, "register.html", gin.H{
			"title":       "Регистрация",
			"error":       "Пароль должен быть не менее 8 символов",
			"username":    username,
		})
      return // Выходим из функции, если данные не валидны.
	}
	// Проверяем, что пароль совпадает с подтверждением пароля
	if password != passwordConfirm {
		c.HTML(http.StatusBadRequest, "register.html", gin.H{
			"title":       "Регистрация",
			"error":       "Пароли не совпадают",
			"username":    username,
		})
		return // Выходим из функции, если данные не валидны.
	}
	// хэшируем пароль перед сохранением его в базе данных
	hashedPassword, err := auth.HashPassword(password)
	if err != nil {
		log.Printf("Ошибка хеширования пароля для пользователя %s: %v",username, err)
		// Если произошла ошибка хеширования, показываем страницу регистрации
		// с сообщением об ошибке и введенным именем пользователя.
		c.HTML(http.StatusInternalServerError, "register.html", gin.H{
			"title":    "Регистрация",
			"error":    "Произошла внутрення ошибка при обработке пороля.",
			"username": username,
		})
		return // Выходим из функции, если произошла ошибка.
	}
	// Создание пользователя в БД
	_, err = database.CreateUser(username, hashedPassword)
	if err != nil {
		log.Printf("Ошибка создания пользователя %s: %v",username, err)
		c.HTML(http.StatusInternalServerError, "register.html", gin.H{
			"title":    "Регистрация",
			"error":    err.Error(),
			"username": username,
		})
		return // Выходим из функции, если произошла ошибка.
	}
	// Если все хорошо, показываем страницу входа с сообщением об успешной регистрации.
	log.Printf("Пользователь %s успешно зарегистрирован.", username)
	c.HTML(http.StatusCreated, "register.html", gin.H{
		"title": "Вход",
		"success": "Вы успешно зарегистрированы! Теперь вы можете войти.",
	})
}
// Обработчик входа
func HandleLogin(c *gin.Context) {
	// 1. Получаем данные из формы
	username := strings.TrimSpace(c.PostForm("username"))
	password := strings.TrimSpace(c.PostForm("password"))
	// Проверяем, что поля не пустые
	if username == "" || password == "" {
		c.HTML(http.StatusBadRequest, "login.html", gin.H{
			"title": "Вход",
			"error": "Имя пользлователя и пароль не могут быть пустыми",
		})
		return
	}
	// Проверяем пользователя в базе данных
	user, err := database.GetUserByUsername(username)
	if err != nil {
		log.Printf("Ошибка получения пользователя %s: %v", username, err)
		c.HTML(http.StatusInternalServerError, "login.html", gin.H{
			"title": "Вход",
			"error": "Ошибка сервера при обработке данных.",
		})
		return
	}
	// Проверяем, что пароль совпадает с паролем в базе данных
	if user == nil || !auth.CheckPasswordHash(password, user.PasswordHash) {
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{
			"title": "Вход",
			"error": "Неверное имя пользователя или пароль.",
		})
		return
	}
	// Если все хорошо, сохраняем данные пользователя в сессии и перенаправляем на главную страницу.
	session := sessions.Default(c)
	// Сохраняем данные пользователя в сессии
	session.Set("userID", user.ID)
	session.Set("username", user.Username)
	err = session.Save()
	if err != nil {
		log.Printf("Ошибка сохранения сессии для пользователя %s: %v", username, err)
		c.HTML(http.StatusInternalServerError, "login.html", gin.H{
			"title": "Вход",
			"error": "Не удалось начать сессию",
		})
		return
	}
	// Показываем страницу входа с сообщением об успешном входе.
	log.Printf("Пользователь %s (ID: %d) успешно вошел в систему.", user.Username, user.ID)
	c.Redirect(http.StatusFound, "/upload")

}
// Обработчик для страницы загрузки изображений
//Доступно для аутентифицированных пользователей
func ShowUploadPage(c *gin.Context) {
	log.Printf("DEBUG: ShowUploadPage получил ЗАПРОС URL: %s", c.Request.URL.String())

	session := sessions.Default(c)
	username := session.Get("username")
	usernameStr, ok := username.(string)
	if !ok {
		usernameStr = "Неизвестный пользователь"
		log.Println("Предупреждение: username не найден в сессии для аутентифицированного пользователя.")
	}
	queryParams := c.Request.URL.Query()
	// --- ИЗМЕНЕНО: Получаем слайсы параметров ---
	// Используем QueryArray для получения всех значений параметра "error"
	errorMessages := queryParams["error"]
	// Используем QueryArray для получения всех значений параметра "success_url"
	successURLs := queryParams["success_url"]
	// --- КОНЕЦ ИЗМЕНЕНИЙ ---

	log.Printf("DEBUG: ShowUploadPage получил success_urls = %v, errors = %v (через QueryArray)", successURLs, errorMessages)

	c.HTML(http.StatusOK, "upload.html", gin.H{
		"title":    "Загрузка изображения",
		"username": usernameStr,
		// Передаем слайсы в шаблон
		"errors":       errorMessages,
		"success_urls": successURLs,
	})
}


func HandleLogout(c *gin.Context) {
	// Выходим из сессии
	session := sessions.Default(c)
	// Удаляем данные пользователя из сессии
	session.Delete("userID")
	session.Delete("username")
	// Устанавливаем время жизни сессии в прошлое, чтобы удалить куки сессии
	session.Options(sessions.Options{MaxAge: -1})
	err := session.Save()
	// Если возникла ошибка при сохранении сессии, то выводим ошибку в лог
	if err != nil {
		log.Printf("Ошибка сохранения сессии после выхода пользователя: %v", err)
	}
	log.Printf("Пользователь успешно вышел из системы.")
	// Перенаправляем пользователя на главную страницу
	c.Redirect(http.StatusFound, "/")
}

func HandleUpload(c *gin.Context) {
	// Проверяем общий размер тела запроса ДО парсинга формы
	// Установим лимит чуть больше, чем MaxFiles * MaxUploadSize, на всякий случай
	maxTotalSize := int64(MaxFiles*MaxUploadSize + 1*1024*1024) // +1MB на заголовки и пр.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxTotalSize)

	// Получаем ID пользователя
	userID, exists := c.Get("userID")
	if !exists {
		log.Println("ОШИБКА: userID не найден в контексте для /upload")
		c.Redirect(http.StatusFound, "/login")
		return
	}
	userID64 := userID.(int64)

	// 1. Парсим multipart форму
	// Передаем максимальный размер данных, хранимых в памяти (остальное во временные файлы)
	err := c.Request.ParseMultipartForm(int64(MaxUploadSize)) // Используем MaxUploadSize на файл
	if err != nil {
		log.Printf("Ошибка парсинга multipart формы для userID %d: %v", userID64, err)
		errorMsg := "Ошибка обработки запроса."
		if err.Error() == "http: request body too large" {
			errorMsg = fmt.Sprintf("Общий размер запроса слишком большой. Максимум около %d MB.", maxTotalSize/1024/1024)
		} else if strings.Contains(err.Error(), "multipart: NextPart") {
             errorMsg = fmt.Sprintf("Ошибка чтения данных файла. Возможно, файл слишком большой (макс. %d MB на файл).", MaxUploadSize/1024/1024)
        }
		redirectWithErrors(c, []string{errorMsg}) // Используем новую функцию редиректа
		return
	}

	// Получаем *слайс* файлов по имени поля "imagefiles"
	files := c.Request.MultipartForm.File["imagefiles"]

	// 2. Проверяем количество файлов
	if len(files) == 0 {
		redirectWithErrors(c, []string{"Вы не выбрали ни одного файла."})
		return
	}
	if len(files) > MaxFiles {
		redirectWithErrors(c, []string{fmt.Sprintf("Можно загрузить не более %d файлов одновременно.", MaxFiles)})
		return
	}

	// 3. Обрабатываем каждый файл в цикле
	var successURLs []string
	var errorMessages []string
	uploadPath := getEnv("UPLOAD_PATH", "/app/uploads")
	for _, fileHeader := range files {
		log.Printf("Обработка файла: %s, Размер: %d (UserID: %d)",
			fileHeader.Filename, fileHeader.Size, userID64)

        // Проверка размера отдельного файла (на всякий случай)
        if fileHeader.Size > MaxUploadSize {
            errorMessages = append(errorMessages, fmt.Sprintf("Файл '%s' слишком большой (макс. %d MB).", fileHeader.Filename, MaxUploadSize/1024/1024))
            continue // Переходим к следующему файлу
        }

		// 3.1. Обработка и сохранение
		storedFilename, err := services.ProcessAndSaveImage(fileHeader, uploadPath)
		if err != nil {
			log.Printf("Ошибка обработки/сохранения файла '%s' для userID %d: %v", fileHeader.Filename, userID64, err)
			errorMessages = append(errorMessages, fmt.Sprintf("Файл '%s': %s", fileHeader.Filename, err.Error()))
			continue // Переходим к следующему файлу
		}

		// 3.2. Генерация токена
		accessToken, err := services.GenerateSecureToken(32)
		if err != nil {
			log.Printf("КРИТИЧЕСКАЯ ОШИБКА: не удалось сгенерировать токен для файла '%s' userID %d: %v", fileHeader.Filename, userID64, err)
			errorMessages = append(errorMessages, fmt.Sprintf("Файл '%s': Внутренняя ошибка сервера (токен).", fileHeader.Filename))
			cleanupFile(storedFilename) // Удаляем уже сохраненный файл
			continue
		}

		// 3.3. Сохранение в БД
		_, err = database.CreateImageRecord(userID64, fileHeader.Filename, storedFilename, accessToken)
		if err != nil {
			log.Printf("КРИТИЧЕСКАЯ ОШИБКА: не удалось сохранить запись в БД для файла '%s' userID %d: %v", fileHeader.Filename, userID64, err)
			errorMessages = append(errorMessages, fmt.Sprintf("Файл '%s': Внутренняя ошибка сервера (БД).", fileHeader.Filename))
			cleanupFile(storedFilename) // Удаляем уже сохраненный файл
			continue
		}

		// 3.4. Формирование URL и добавление в список успешных
		baseURL := os.Getenv("BASE_URL")
		if baseURL == "" {
			baseURL = "http://localhost:8080" // Fallback
			log.Printf("ПРЕДУПРЕЖДЕНИЕ: Переменная окружения BASE_URL не установлена, используется fallback '%s'", baseURL)
		}
		viewURL := fmt.Sprintf("%s/view/%s", baseURL, accessToken)
		successURLs = append(successURLs, viewURL)
		log.Printf("Файл '%s' успешно обработан userID %d. URL: %s", fileHeader.Filename, userID64, viewURL)
	} // Конец цикла по файлам

	// 4. Формируем URL для редиректа с результатами
	redirectURL := buildRedirectURL("/upload", successURLs, errorMessages)

	log.Printf("Завершена обработка %d файлов для userID %d. Успешно: %d, Ошибки: %d. Редирект на: %s",
		len(files), userID64, len(successURLs), len(errorMessages), redirectURL)

	c.Redirect(http.StatusFound, redirectURL)
}

func redirectWithErrors(c *gin.Context, errors []string) {
    redirectURL := buildRedirectURL("/upload", nil, errors) // Передаем только ошибки
    c.Redirect(http.StatusFound, redirectURL)
}
// Вспомогательная функция для очистки файла при ошибке

func cleanupFile(filename string) {
	uploadPath := getEnv("UPLOAD_PATH", "/app/uploads")
	if filename != "" {
		err := os.Remove(filepath.Join(uploadPath, filename))
		if err != nil {
			log.Printf("ПРЕДУПРЕЖДЕНИЕ: не удалось удалить файл %s после ошибки: %v", filename, err)
		} else {
			log.Printf("Файл %s удален после ошибки.", filename)
		}
	}
}

// ShowConfirmViewPage обрабатывает GET-запрос на просмотр изображения.
// Проверяет токен и статус, показывает страницу подтверждения.
func ShowConfirmViewPage(c *gin.Context) {
	// Получаем токен из параметра URL (:token)
	token := c.Param("token")
	if token == "" {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"message": "Отсутствует токен доступа."})
		return
	}

	// Ищем изображение по токену в БД
	img, err := database.GetImageByToken(token)
	if err != nil {
		log.Printf("Ошибка БД при поиске токена %s: %v", token, err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"message": "Ошибка сервера при поиске изображения."})
		return
	}

	// Проверяем, найдено ли изображение
	if img == nil {
		log.Printf("Токен %s не найден в БД.", token)
		c.HTML(http.StatusNotFound, "error.html", gin.H{"message": "Ссылка недействительна или устарела."})
		return
	}

	// Проверяем статус изображения. Показать подтверждение можно только для 'pending'.
	if img.Status != "pending" {
		log.Printf("Попытка доступа к уже просмотренному/удаленному токену %s (статус: %s)", token, img.Status)
		// Используем код 410 Gone, чтобы указать, что ресурс был здесь, но теперь недоступен навсегда
		c.HTML(http.StatusGone, "error.html", gin.H{"message": "Эта ссылка уже была использована или истекла."})
		return
	}
	// Все проверки пройдены, показываем страницу подтверждения
	c.HTML(http.StatusOK, "confirm_view.html", gin.H{
		"title": "Подтверждение просмотра",
		"token": token, // Передаем токен в шаблон для POST-формы
	})
}

// HandleConfirmView обрабатывает POST-запрос подтверждения просмотра.
// Отдает изображение и помечает его как просмотренное.
func HandleConfirmView(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		// Отвечаем JSON, так как это POST запрос, может быть вызван не из браузера
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Отсутствует токен доступа."})
		return
	}

	// 1. Повторно ищем изображение в БД (защита от race condition)
	img, err := database.GetImageByToken(token)
	if err != nil {
		log.Printf("Ошибка БД при повторном поиске токена %s (POST): %v", token, err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Ошибка сервера при поиске изображения."})
		return
	}

	// 2. Повторно проверяем, что изображение найдено и в статусе 'pending'
	if img == nil || img.Status != "pending" {
		log.Printf("Повторный доступ или race condition для токена %s (POST, статус: %s)", token, img.Status)
		c.AbortWithStatusJSON(http.StatusGone, gin.H{"error": "Ссылка недействительна или уже использована."})
		return
	}

	// 3. Помечаем изображение как просмотренное в БД
	err = database.MarkImageViewed(token)
	if err != nil {
		// Эта ошибка может возникнуть, если MarkImageViewed не обновил строку (например, статус изменился между SELECT и UPDATE)
		// или при другой ошибке БД.
		log.Printf("Не удалось пометить токен %s как просмотренный: %v", token, err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Не удалось обработать запрос на просмотр."})
		return
	}

	// 4. Отправляем файл пользователю
	uploadPath := getEnv("UPLOAD_PATH", "/app/uploads")
	filePath := filepath.Join(uploadPath, img.StoredFilename)

	// Устанавливаем заголовки для предотвращения кэширования
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")

	// Отправляем файл. Gin установит Content-Type, если он известный.
	// c.File() сама обрабатывает ошибки чтения файла и т.д.
	c.File(filePath)
	// Важно: после c.File() дальнейший код в хендлере может не выполниться,
	// если соединение оборвется.

	// 5. Логика удаления файла (после успешной отправки!)
	//    ВНИМАНИЕ: Удаление сразу после c.File() не гарантировано.
	//    Лучше использовать фоновый процесс. Но для простоты покажем прямой вызов.

	// Запускаем удаление в отдельной горутине, чтобы не блокировать ответ клиенту
	// и дать c.File() завершиться.
	go func(pathToDelete string, imageID int64) {
		err := os.Remove(pathToDelete)
		if err != nil {
			log.Printf("ОШИБКА: не удалось удалить файл %s (ImageID: %d): %v", pathToDelete, imageID, err)
			// Можно добавить логику повторной попытки или пометить в БД как 'delete_failed'
			// _ = database.UpdateImageStatus(imageID, "delete_failed")
		} else {
			log.Printf("Файл %s успешно удален (ImageID: %d).", pathToDelete, imageID)
			// Опционально: обновить статус в БД на 'deleted'
			// _ = database.UpdateImageStatus(imageID, "deleted")
		}
	}(filePath, img.ID) // Передаем путь и ID в горутину

}

// buildRedirectURL формирует URL для редиректа с параметрами success_url и error.
func buildRedirectURL(basePath string, successURLs []string, errorMessages []string) string {
	query := url.Values{} // Используем url.Values для корректного добавления параметров
	for _, sURL := range successURLs {
		query.Add("success_url", sURL) // Добавляем каждый URL как отдельный параметр
	}
	for _, errMsg := range errorMessages {
		query.Add("error", errMsg) // Добавляем каждую ошибку как отдельный параметр
	}

	// Собираем URL
	redirectURL := basePath
	if queryString := query.Encode(); queryString != "" { // Encode() сама кодирует значения
		redirectURL += "?" + queryString
	}
	return redirectURL
}