package handlers

import (
	// Стандартные библиотеки
	"fmt"      // Для форматирования строк (ошибок, URL)
	"log"      // Для логирования
	"net/http" // Для кодов статуса HTTP и заголовков
	"os"       // Для работы с файлами (проверка, удаление)
	"path/filepath" // Для работы с путями к файлам
	"strings"  // Для работы со строками (TrimSpace, Contains)
	"time"     // Для задержки перед удалением файла

	// Внутренние пакеты
	"imagecleaner/internal/auth"     // Для проверки хеша пароля
	"imagecleaner/internal/database" // Для взаимодействия с БД
	"imagecleaner/internal/services" // Для обработки изображений и генерации токенов

	// Сторонние библиотеки
	"github.com/gin-contrib/sessions" // Для работы с сессиями
	"github.com/gin-gonic/gin"        // Основной фреймворк
)

// Константы для ограничений загрузки
const MaxUploadSize = 10 << 20 // 10 МБ (10 * 2^20 байт) - максимальный размер одного файла
const MaxFiles = 10            // Максимальное количество файлов в одной загрузке

// getEnv - локальная вспомогательная функция для получения переменных окружения.
// Используется для получения UPLOAD_PATH и BASE_URL.
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	// Лог не нужен здесь, т.к. он уже есть в main.go при старте
	return fallback
}

// ShowLoginPage отображает страницу входа.
// Также обрабатывает flash-сообщения (ошибки/успех), переданные из других обработчиков.
func ShowLoginPage(c *gin.Context) {
	session := sessions.Default(c) // Получаем доступ к сессии

	// Пытаемся получить flash-сообщения из сессии
	errorMsg := session.Get("error")
	successMsg := session.Get("success")

	// Удаляем flash-сообщения из сессии после их прочтения, чтобы они не отображались повторно
	session.Delete("error")
	session.Delete("success")
	err := session.Save() // Сохраняем сессию, чтобы удаления применились
	if err != nil {
		log.Printf("Ошибка сохранения сессии при очистке flash в ShowLoginPage: %v", err)
	}

	// Рендерим HTML-шаблон login.html, передавая в него данные
	c.HTML(http.StatusOK, "login.html", gin.H{
		"title":   "Вход",     // Заголовок страницы
		"error":   errorMsg,   // Сообщение об ошибке (если было)
		"success": successMsg, // Сообщение об успехе (если было)
	})
}

// ShowRegisterPage отображает страницу регистрации.
// Обрабатывает flash-сообщения и предзаполняет поле username, если была ошибка.
func ShowRegisterPage(c *gin.Context) {
	session := sessions.Default(c)

	// Получаем flash-сообщения
	errorMsg := session.Get("error")
	successMsg := session.Get("success")
	// Получаем введенное ранее имя пользователя (если была ошибка и мы его сохранили)
	usernameValue := session.Get("username_input")

	// Очищаем flash-сообщения и сохраненное имя пользователя
	session.Delete("error")
	session.Delete("success")
	session.Delete("username_input")
	err := session.Save()
	if err != nil {
		log.Printf("Ошибка сохранения сессии при очистке flash в ShowRegisterPage: %v", err)
	}

	// Рендерим шаблон register.html
	c.HTML(http.StatusOK, "register.html", gin.H{
		"title":    "Регистрация",
		"error":    errorMsg,
		"success":  successMsg,
		"username": usernameValue, // Передаем имя пользователя для предзаполнения поля
	})
}

// HandleRegister обрабатывает POST-запрос с формы регистрации.
func HandleRegister(c *gin.Context) {
	// Получаем данные из формы и удаляем лишние пробелы по краям
	username := strings.TrimSpace(c.PostForm("username"))
	password := strings.TrimSpace(c.PostForm("password"))
	passwordConfirm := strings.TrimSpace(c.PostForm("password_confirm"))

	session := sessions.Default(c)

	// Вспомогательная функция для редиректа с сообщением об ошибке
	redirectWithError := func(message string) {
		session.Set("error", message)           // Устанавливаем flash-сообщение об ошибке
		session.Set("username_input", username) // Сохраняем введенное имя пользователя
		err := session.Save()
		if err != nil {
			log.Printf("Ошибка сохранения сессии при редиректе с ошибкой в HandleRegister: %v", err)
			// Если сессию не сохранить, flash не передастся. Можно показать страницу ошибки или просто редиректнуть.
		}
		c.Redirect(http.StatusFound, "/register") // Перенаправляем обратно на страницу регистрации (GET)
	}

	// --- Валидация входных данных ---
	if username == "" || password == "" || passwordConfirm == "" {
		redirectWithError("Все поля должны быть заполнены")
		return // Прекращаем обработку
	}
	if len(password) < 8 {
		redirectWithError("Пароль должен быть не менее 8 символов")
		return
	}
	if password != passwordConfirm {
		redirectWithError("Пароли не совпадают")
		return
	}

	// --- Создание пользователя ---
	// Хешируем пароль перед сохранением
	hashedPassword, err := auth.HashPassword(password)
	if err != nil {
		log.Printf("Ошибка хеширования пароля для пользователя %s: %v", username, err)
		redirectWithError("Произошла внутренняя ошибка при обработке пароля.")
		return
	}

	// Пытаемся создать пользователя в базе данных
	_, err = database.CreateUser(username, hashedPassword)
	if err != nil {
		log.Printf("Ошибка создания пользователя %s: %v", username, err)
		// Проверяем, является ли ошибка ошибкой уникальности имени пользователя
		// Используем кастомную ошибку из database.CreateUser или проверяем текст ошибки
		if strings.Contains(err.Error(), "уже существует") { // Предполагаем, что CreateUser возвращает такую ошибку
			redirectWithError(err.Error()) // Показываем пользователю сообщение об уникальности
		} else {
			redirectWithError("Произошла внутренняя ошибка при создании пользователя.") // Общая ошибка
		}
		return
	}

	// --- Успешная регистрация ---
	log.Printf("Пользователь %s успешно зарегистрирован.", username)
	// Устанавливаем flash-сообщение об успехе для страницы входа
	session.Set("success", "Вы успешно зарегистрированы! Теперь вы можете войти.")
	err = session.Save()
	if err != nil {
		log.Printf("Ошибка сохранения сессии после успешной регистрации %s: %v", username, err)
	}
	// Перенаправляем на страницу входа
	c.Redirect(http.StatusFound, "/login")
}

// HandleLogin обрабатывает POST-запрос с формы входа.
func HandleLogin(c *gin.Context) {
	// Получаем данные из формы
	username := strings.TrimSpace(c.PostForm("username"))
	password := strings.TrimSpace(c.PostForm("password"))

	session := sessions.Default(c)

	// Вспомогательная функция для редиректа с ошибкой
	redirectWithError := func(message string) {
		session.Set("error", message)
		err := session.Save()
		if err != nil {
			log.Printf("Ошибка сохранения сессии при редиректе с ошибкой в HandleLogin: %v", err)
		}
		c.Redirect(http.StatusFound, "/login") // Редирект на страницу входа (GET)
	}

	// --- Валидация ---
	if username == "" || password == "" {
		redirectWithError("Имя пользователя и пароль не могут быть пустыми")
		return
	}

	// --- Проверка учетных данных ---
	// Ищем пользователя в БД по имени
	user, err := database.GetUserByUsername(username)
	if err != nil {
		// Ошибка при запросе к БД
		log.Printf("Ошибка получения пользователя %s из БД: %v", username, err)
		redirectWithError("Ошибка сервера при проверке данных.")
		return
	}

	// Проверяем, найден ли пользователь и совпадает ли хеш пароля
	if user == nil || !auth.CheckPasswordHash(password, user.PasswordHash) {
		log.Printf("Неудачная попытка входа для пользователя '%s': неверный пароль или пользователь не найден.", username)
		redirectWithError("Неверное имя пользователя или пароль.")
		return
	}

	// --- Успешный вход ---
	// Обновляем сессию пользователя. Важно сохранять новые данные (ID, имя).
	// Gin-contrib-sessions обычно регенерирует ID сессии при изменении данных и сохранении,
	// что помогает предотвратить атаку фиксации сессии (session fixation).
	session.Set("userID", user.ID)
	session.Set("username", user.Username)
	err = session.Save() // Сохраняем обновленную сессию
	if err != nil {
		log.Printf("Ошибка сохранения сессии после успешного входа пользователя %s (ID: %d): %v", username, user.ID, err)
		redirectWithError("Не удалось сохранить данные сессии.")
		return
	}

	log.Printf("Пользователь %s (ID: %d) успешно вошел в систему.", user.Username, user.ID)
	// Перенаправляем на защищенную страницу загрузки
	c.Redirect(http.StatusFound, "/upload")
}

// ShowUploadPage отображает страницу загрузки изображений.
// Показывает имя пользователя и результаты предыдущей загрузки (из flash-сообщений).
func ShowUploadPage(c *gin.Context) {
	session := sessions.Default(c)
	username := session.Get("username") // Получаем имя пользователя из сессии
	usernameStr, _ := username.(string) // Преобразуем в строку, игнорируя ошибку типа

	// Получаем flash-сообщения о результатах предыдущей загрузки
	errorsFlash := session.Get("upload_errors")
	successURLsFlash := session.Get("upload_success_urls")

	// Очищаем flash-сообщения из сессии
	session.Delete("upload_errors")
	session.Delete("upload_success_urls")
	err := session.Save()
	if err != nil {
		log.Printf("Ошибка сохранения сессии при очистке flash в ShowUploadPage: %v", err)
	}

	// Преобразуем flash-сообщения (которые имеют тип interface{}) в нужные слайсы строк
	var errorMessages []string
	if errs, ok := errorsFlash.([]string); ok { // Проверяем тип перед использованием
		errorMessages = errs
	}
	var successURLs []string
	if urls, ok := successURLsFlash.([]string); ok {
		successURLs = urls
	}

	// Рендерим шаблон upload.html
	c.HTML(http.StatusOK, "upload.html", gin.H{
		"title":        "Загрузка изображения",
		"username":     usernameStr,
		"errors":       errorMessages, // Передаем слайс ошибок
		"success_urls": successURLs,   // Передаем слайс успешных URL
	})
}

// HandleLogout обрабатывает выход пользователя из системы.
func HandleLogout(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("userID") // Получаем ID для логирования перед удалением

	// Удаляем данные пользователя из сессии
	session.Delete("userID")
	session.Delete("username")
	// Устанавливаем MaxAge в -1, чтобы браузер удалил cookie сессии
	session.Options(sessions.Options{MaxAge: -1})
	err := session.Save() // Сохраняем изменения
	if err != nil {
		log.Printf("Ошибка сохранения сессии после выхода пользователя (ID: %v): %v", userID, err)
		// Несмотря на ошибку, все равно перенаправляем на главную
	} else {
		log.Printf("Пользователь (ID: %v) успешно вышел из системы.", userID)
	}
	// Перенаправляем на главную (публичную) страницу
	c.Redirect(http.StatusFound, "/")
}

// HandleUpload обрабатывает загрузку файлов изображений.
func HandleUpload(c *gin.Context) {
	// --- Предварительные проверки ---
	// 1. Проверяем общий размер тела запроса, чтобы предотвратить DoS атаки большими запросами.
	//    Устанавливаем лимит немного больше, чем максимально возможный размер всех файлов + заголовки.
	maxTotalSize := int64(MaxFiles*MaxUploadSize + 1*1024*1024) // +1MB на заголовки и метаданные формы
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxTotalSize)

	// 2. Получаем ID пользователя из контекста (установлен middleware AuthRequired)
	userID, exists := c.Get("userID")
	if !exists {
		// Это не должно происходить, если middleware работает правильно, но проверяем на всякий случай
		log.Println("КРИТИЧЕСКАЯ ОШИБКА: userID не найден в контексте для /upload, хотя middleware AuthRequired должен был его установить.")
		c.Redirect(http.StatusFound, "/login") // Отправляем на вход
		return
	}
	userID64, ok := userID.(int64)
	if !ok {
		log.Printf("КРИТИЧЕСКАЯ ОШИБКА: Некорректный тип userID (%T) в контексте для /upload.", userID)
		c.Redirect(http.StatusFound, "/login")
		return
	}

	// --- Парсинг формы ---
	// 3. Парсим multipart/form-data. Указываем лимит памяти на *файл* (MaxUploadSize).
	//    Файлы больше этого лимита будут сохраняться во временные файлы на диске.
	err := c.Request.ParseMultipartForm(int64(MaxUploadSize))
	if err != nil {
		// Обрабатываем ошибки парсинга
		log.Printf("Ошибка парсинга multipart формы для userID %d: %v", userID64, err)
		errorMsg := "Ошибка обработки запроса при загрузке файлов."
		// Проверяем специфические ошибки
		if err.Error() == "http: request body too large" {
			// Эта ошибка возникает, если общий размер запроса превысил лимит MaxBytesReader
			errorMsg = fmt.Sprintf("Общий размер запроса слишком большой. Максимум около %d MB.", maxTotalSize/1024/1024)
		} else if strings.Contains(err.Error(), "multipart: NextPart") || strings.Contains(err.Error(), "unexpected EOF") || strings.Contains(err.Error(),"EOF") {
			// Эти ошибки могут возникать при чтении частей формы, часто если отдельный файл слишком большой
			// или произошла ошибка при передаче данных.
			errorMsg = fmt.Sprintf("Ошибка чтения данных файла. Возможно, один из файлов слишком большой (макс. %d MB на файл) или произошла ошибка передачи.", MaxUploadSize/1024/1024)
		}
		// Используем flash-сообщения для передачи ошибки на страницу /upload
		saveFlashAndRedirectUpload(c, nil, []string{errorMsg})
		return
	}

	// --- Получение и проверка файлов ---
	// 4. Получаем *слайс* файлов из формы по имени поля "imagefiles".
	files := c.Request.MultipartForm.File["imagefiles"]

	// 5. Проверяем количество файлов.
	if len(files) == 0 {
		saveFlashAndRedirectUpload(c, nil, []string{"Вы не выбрали ни одного файла."})
		return
	}
	if len(files) > MaxFiles {
		saveFlashAndRedirectUpload(c, nil, []string{fmt.Sprintf("Можно загрузить не более %d файлов одновременно.", MaxFiles)})
		return
	}

	// --- Обработка каждого файла ---
	var successURLs []string     // Слайс для хранения URL успешно загруженных файлов
	var errorMessages []string // Слайс для хранения сообщений об ошибках для каждого файла

	// Получаем пути и базовый URL из переменных окружения
	uploadPath := getEnv("UPLOAD_PATH", "/app/uploads")
	baseURL := getEnv("BASE_URL", "") // BASE_URL критичен для генерации ссылок

	// Проверяем, установлен ли BASE_URL
	if baseURL == "" {
		log.Printf("КРИТИЧЕСКАЯ ОШИБКА КОНФИГУРАЦИИ: Переменная окружения BASE_URL не установлена! Ссылки для просмотра не будут сгенерированы.")
		// Добавляем общее сообщение об ошибке, которое увидит пользователь
		errorMessages = append(errorMessages, "Ошибка конфигурации сервера: невозможно сгенерировать ссылки для просмотра.")
		// Не прерываем цикл, чтобы обработать файлы, но ссылки не будут созданы.
	}

	// 6. Итерируемся по каждому полученному файлу
	for _, fileHeader := range files {
		log.Printf("Обработка файла: %s, Размер: %d байт (UserID: %d)",
			fileHeader.Filename, fileHeader.Size, userID64)

		// 6.1 Дополнительная проверка размера и пустого файла
		if fileHeader.Size == 0 {
			errorMessages = append(errorMessages, fmt.Sprintf("Файл '%s' пустой и не будет обработан.", fileHeader.Filename))
			continue // Переходим к следующему файлу
		}
		if fileHeader.Size > MaxUploadSize {
			errorMessages = append(errorMessages, fmt.Sprintf("Файл '%s' слишком большой (%.2f MB > %.2f MB).", fileHeader.Filename, float64(fileHeader.Size)/1024/1024, float64(MaxUploadSize)/1024/1024))
			continue // Переходим к следующему файлу
		}

		// 6.2 Обработка и сохранение изображения с помощью сервиса
		// storedFilename - это только имя файла (например, abcdef12345.png)
		storedFilename, err := services.ProcessAndSaveImage(fileHeader, uploadPath)
		if err != nil {
			// Ошибка при обработке или сохранении файла
			log.Printf("Ошибка обработки/сохранения файла '%s' для userID %d: %v", fileHeader.Filename, userID64, err)
			// Формируем сообщение об ошибке для пользователя
			errMsg := "Ошибка обработки файла." // Общее сообщение
			if strings.Contains(err.Error(), "Недопустимый тип файла") {
				errMsg = "Недопустимый тип файла (разрешены JPEG, PNG, GIF)."
			} else if strings.Contains(err.Error(), "Не удалось декодировать") {
				errMsg = "Не удалось распознать формат файла или файл поврежден."
			} else if strings.Contains(err.Error(), "не удалось создать файл") {
                 errMsg = "Внутренняя ошибка сервера при сохранении файла."
            }
			errorMessages = append(errorMessages, fmt.Sprintf("Файл '%s': %s", fileHeader.Filename, errMsg))
			continue // Переходим к следующему файлу
		}

		// 6.3 Генерация уникального токена доступа для ссылки
		accessToken, err := services.GenerateSecureToken(32) // 32 байта = ~43 символа base64
		if err != nil {
			// Критическая ошибка, если не удается сгенерировать токен
			log.Printf("КРИТИЧЕСКАЯ ОШИБКА: не удалось сгенерировать токен для файла '%s' (сохранен как %s) userID %d: %v", fileHeader.Filename, storedFilename, userID64, err)
			errorMessages = append(errorMessages, fmt.Sprintf("Файл '%s': Внутренняя ошибка сервера (не удалось создать токен).", fileHeader.Filename))
			// Нужно удалить уже сохраненный файл, т.к. для него нет токена
			cleanupFile(filepath.Join(uploadPath, storedFilename))
			continue // Переходим к следующему файлу
		}

		// 6.4 Сохранение информации об изображении в базу данных
		imageID, err := database.CreateImageRecord(userID64, fileHeader.Filename, storedFilename, accessToken)
		if err != nil {
			// Критическая ошибка, если не удалось сохранить запись в БД
			log.Printf("КРИТИЧЕСКАЯ ОШИБКА: не удалось сохранить запись в БД для файла '%s' (токен %s, сохранен как %s) userID %d: %v", fileHeader.Filename, accessToken, storedFilename, userID64, err)
			errMsg := "Внутренняя ошибка сервера (не удалось сохранить информацию о файле)."
            if strings.Contains(err.Error(), "конфликт") { // Обработка ошибки уникальности из CreateImageRecord
                 errMsg = "Внутренняя ошибка сервера (конфликт данных, попробуйте еще раз)."
            }
			errorMessages = append(errorMessages, fmt.Sprintf("Файл '%s': %s", fileHeader.Filename, errMsg))
			// Нужно удалить сохраненный файл, т.к. запись в БД не удалась
			cleanupFile(filepath.Join(uploadPath, storedFilename))
			continue // Переходим к следующему файлу
		}

		// 6.5 Формирование URL для просмотра (только если BASE_URL установлен)
		if baseURL != "" {
			viewURL := fmt.Sprintf("%s/view/%s", baseURL, accessToken)
			successURLs = append(successURLs, viewURL) // Добавляем URL в список успешных
			log.Printf("Файл '%s' (ID: %d) успешно обработан userID %d. URL: %s", fileHeader.Filename, imageID, userID64, viewURL)
		} else {
			// Если BASE_URL не задан, файл обработан, но URL не можем показать
			log.Printf("Файл '%s' (ID: %d) успешно обработан userID %d, но URL не сформирован (BASE_URL не задан).", fileHeader.Filename, imageID, userID64)
			// Можно добавить сообщение об успехе без URL или ошибку конфигурации
			// errorMessages = append(errorMessages, fmt.Sprintf("Файл '%s': Успешно загружен, но URL не может быть сформирован (ошибка конфигурации сервера).", fileHeader.Filename))
		}

	} // Конец цикла for по файлам

	// --- Завершение обработки и редирект ---
	log.Printf("Завершена обработка %d файлов для userID %d. Успешно с URL: %d, Ошибки: %d.",
		len(files), userID64, len(successURLs), len(errorMessages))

	// Сохраняем результаты (URLы и ошибки) во flash-сообщения сессии
	saveFlashAndRedirectUpload(c, successURLs, errorMessages)
}

// saveFlashAndRedirectUpload сохраняет результаты (успешные URL и ошибки)
// во flash-сообщения сессии и перенаправляет пользователя обратно на страницу загрузки.
func saveFlashAndRedirectUpload(c *gin.Context, successURLs []string, errorMessages []string) {
	session := sessions.Default(c) // Получаем сессию
	// Сохраняем данные во flash, только если они не пустые
	if len(successURLs) > 0 {
		session.Set("upload_success_urls", successURLs)
	}
	if len(errorMessages) > 0 {
		session.Set("upload_errors", errorMessages)
	}
	// Сохраняем сессию, чтобы flash-сообщения записались
	err := session.Save()
	if err != nil {
		log.Printf("Ошибка сохранения сессии при записи flash-сообщений в saveFlashAndRedirectUpload: %v", err)
		// Если сессию не сохранить, пользователь не увидит результаты.
		// Можно показать страницу с ошибкой или просто редиректнуть.
	}
	// Перенаправляем пользователя на GET /upload, где flash-сообщения будут отображены
	c.Redirect(http.StatusFound, "/upload")
}

// cleanupFile - вспомогательная функция для удаления файла по полному пути.
// Используется для очистки после ошибок при обработке загрузки.
func cleanupFile(fullPath string) {
	if fullPath != "" { // Проверяем, что путь не пустой
		log.Printf("Попытка удаления файла %s из-за ошибки...", fullPath)
		err := os.Remove(fullPath) // Удаляем файл
		if err != nil {
			// Если удалить не удалось (например, нет прав или файл уже удален),
			// логируем предупреждение, но не останавливаем выполнение.
			log.Printf("ПРЕДУПРЕЖДЕНИЕ: не удалось удалить файл %s после ошибки: %v", fullPath, err)
		} else {
			log.Printf("Файл %s успешно удален после ошибки обработки.", fullPath)
		}
	}
}

// ShowConfirmViewPage отображает страницу подтверждения перед показом изображения.
func ShowConfirmViewPage(c *gin.Context) {
	// Получаем токен из параметра URL (:token)
	token := c.Param("token")
	if token == "" {
		// Если токена нет в URL (маловероятно из-за роутинга, но проверяем)
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"title": "Ошибка запроса", "message": "Отсутствует идентификатор изображения в ссылке."})
		return
	}

	// Ищем изображение по токену в базе данных
	img, err := database.GetImageByToken(token)
	if err != nil {
		// Ошибка при взаимодействии с БД
		log.Printf("Ошибка БД при поиске токена %s в ShowConfirmViewPage: %v", token, err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"title": "Ошибка сервера", "message": "Произошла ошибка при поиске информации об изображении."})
		return
	}

	// Проверяем, найдено ли изображение
	if img == nil {
		// Токен не найден в БД
		log.Printf("Токен %s не найден в БД (GET /view).", token)
		c.HTML(http.StatusNotFound, "error.html", gin.H{"title": "Не найдено", "message": "Ссылка недействительна или устарела."})
		return
	}

	// Проверяем статус изображения. Показать подтверждение можно только для статуса 'pending'.
	if img.Status != "pending" {
		// Если статус другой (например, 'viewed' или 'deleted'), ссылка уже использована.
		log.Printf("Попытка доступа (GET /view) к уже использованному токену %s (статус: %s)", token, img.Status)
		// Используем код 410 Gone, чтобы указать, что ресурс был здесь, но теперь недоступен навсегда.
		c.HTML(http.StatusGone, "error.html", gin.H{"title": "Ссылка истекла", "message": "Эта ссылка уже была использована или срок её действия истёк."})
		return
	}

	// Все проверки пройдены, показываем страницу подтверждения.
	c.HTML(http.StatusOK, "confirm_view.html", gin.H{
		"title": "Подтверждение просмотра",
		"token": token, // Передаем токен в шаблон, он будет использован в action формы
	})
}

// HandleConfirmView обрабатывает POST-запрос со страницы подтверждения.
// Помечает изображение как просмотренное, отдает файл и запускает его удаление.
func HandleConfirmView(c *gin.Context) {
	// Получаем токен из URL
	token := c.Param("token")
	if token == "" {
		// Отвечаем JSON, т.к. это может быть не только браузерный запрос (хотя форма шлет POST)
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Отсутствует токен доступа в URL."})
		return
	}

	// --- Повторные проверки (Защита от Race Condition и изменений) ---
	// 1. Повторно ищем изображение в БД. Важно на случай, если статус изменился
	//    между загрузкой страницы подтверждения (GET) и отправкой формы (POST).
	img, err := database.GetImageByToken(token)
	if err != nil {
		log.Printf("Ошибка БД при повторном поиске токена %s (POST /view): %v", token, err)
		// Отвечаем HTML ошибкой, так как запрос пришел с HTML страницы
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"title": "Ошибка сервера", "message": "Ошибка сервера при проверке ссылки."})
		c.Abort() // Прерываем выполнение
		return
	}

	// 2. Повторно проверяем, что изображение найдено и все еще в статусе 'pending'.
	if img == nil || img.Status != "pending" {
		log.Printf("Попытка повторного доступа (POST /view) или race condition для токена %s (статус: %s)", token, img.Status)
		c.HTML(http.StatusGone, "error.html", gin.H{"title": "Ссылка истекла", "message": "Ссылка недействительна или уже была использована."})
		c.Abort()
		return
	}

	// --- Основная логика ---
	// 3. Помечаем изображение как просмотренное в БД.
	//    MarkImageViewed обновляет статус только если он 'pending', что дает дополнительную атомарную проверку.
	err = database.MarkImageViewed(token)
	if err != nil {
		// Эта ошибка может возникнуть, если MarkImageViewed не обновил строку (например, статус изменился между SELECT и UPDATE)
		// или при другой ошибке БД.
		log.Printf("Не удалось пометить токен %s как просмотренный (ImageID: %d): %v", token, img.ID, err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"title": "Ошибка сервера", "message": "Не удалось обработать ваш запрос на просмотр."})
		c.Abort()
		return
	}
	// Если ошибки нет, значит статус успешно обновлен на 'viewed'
	log.Printf("Токен %s успешно помечен как 'viewed' в БД (ImageID: %d) перед отправкой файла", token, img.ID)

	// 4. Готовимся отправить файл пользователю.
	uploadPath := getEnv("UPLOAD_PATH", "/app/uploads")
	filePath := filepath.Join(uploadPath, img.StoredFilename) // Формируем полный путь к файлу

	// 4.1 Проверяем, существует ли файл на диске перед отправкой.
	//     Это важно на случай, если файл был удален вручную или произошел сбой ранее.
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		log.Printf("КРИТИЧЕСКАЯ ОШИБКА: Файл %s не найден на диске для токена %s (ImageID: %d), хотя запись в БД помечена как 'viewed'!", filePath, token, img.ID)
		// Можно попытаться обновить статус в БД на 'file_missing_error'
		// database.UpdateImageStatus(img.ID, "file_missing_error")
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"title": "Ошибка сервера", "message": "Ошибка: файл изображения не найден на сервере."})
		c.Abort()
		return
	} else if err != nil {
		// Другая ошибка при доступе к файлу (например, права доступа)
		log.Printf("КРИТИЧЕСКАЯ ОШИБКА: Ошибка доступа к файлу %s для токена %s (ImageID: %d): %v", filePath, token, img.ID, err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"title": "Ошибка сервера", "message": "Ошибка доступа к файлу изображения на сервере."})
		c.Abort()
		return
	}

	// 4.2 Устанавливаем заголовки для предотвращения кэширования файла браузером или прокси.
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate, max-age=0")
	c.Header("Pragma", "no-cache") // Для HTTP/1.0 прокси
	c.Header("Expires", "0")       // Для старых кешей

	log.Printf("Отправка файла %s клиенту (Token: %s, ImageID: %d)", filePath, token, img.ID)
	// 4.3 Отправляем файл. Gin сам определяет Content-Type по расширению файла.
	//     c.File() блокирует выполнение до завершения отправки или ошибки.
	c.File(filePath)
	// ВАЖНО: После c.File() выполнение хендлера может продолжаться, если соединение не оборвалось.

	// 5. Запускаем удаление файла в отдельной горутине ПОСЛЕ начала отправки файла.
	//    Это делается асинхронно, чтобы не блокировать завершение ответа клиенту.
	//    Есть риск, что файл будет удален до того, как клиент его полностью скачает,
	//    особенно при медленном соединении. Небольшая задержка немного снижает этот риск.
	//    Более надежное решение - использовать фоновый воркер (worker queue).
	go func(pathToDelete string, imageID int64, tokenToDelete string) {
		// Небольшая задержка перед удалением
		time.Sleep(2 * time.Second)

		log.Printf("Попытка асинхронного удаления файла %s для ImageID: %d (Token: %s) после просмотра", pathToDelete, imageID, tokenToDelete)
		err := os.Remove(pathToDelete) // Пытаемся удалить файл
		if err != nil {
			// Если удалить не удалось, логируем ошибку
			log.Printf("ОШИБКА АСИНХРОННОГО УДАЛЕНИЯ ФАЙЛА: не удалось удалить файл %s (ImageID: %d, Token: %s): %v", pathToDelete, imageID, tokenToDelete, err)
			// Можно обновить статус в БД на 'delete_failed', чтобы потом обработать вручную или другим процессом
			// errUpdate := database.UpdateImageStatus(imageID, "delete_failed")
			// if errUpdate != nil { log.Printf("Ошибка обновления статуса на delete_failed для ImageID %d: %v", imageID, errUpdate) }
		} else {
			// Файл успешно удален
			log.Printf("Файл %s успешно удален асинхронно после просмотра (ImageID: %d, Token: %s).", pathToDelete, imageID, tokenToDelete)
			// Опционально: обновить статус в БД на 'deleted' для полной картины
			// errUpdate := database.UpdateImageStatus(imageID, "deleted")
			// if errUpdate != nil { log.Printf("Ошибка обновления статуса на deleted для ImageID %d: %v", imageID, errUpdate) }
		}
		// Горутина завершает свою работу
	}(filePath, img.ID, token) // Передаем копии переменных в горутину

	// Основной хендлер завершает свою работу здесь, не дожидаясь завершения горутины.
}