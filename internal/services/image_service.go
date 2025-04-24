package services

import (
	"fmt"
	"image"
	"image/gif"
	_ "image/gif"
	"image/jpeg"
	_ "image/jpeg"
	"image/png"
	_ "image/png"

	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// UploadPath - путь к папке для сохранения загруженных файлов.
// AllowedImageTypes - карта разрешенных MIME-типов изображений.
var AllowebImageTypes = map[string]bool{
	"image/jpeg":  true,
	"image/png":   true,
	"image/gif":   true,
}

func ProcessAndSaveImage(fileHeader *multipart.FileHeader, uploadDir string) (storedFilename string, err error) {
	//Открываем загруженный файл
	file, err := fileHeader.Open()
	if err != nil {
		return "", fmt.Errorf("Не удалось открыть загруженный файл: %w", err)
	}
	defer file.Close() //закрыли файл после использования
	//Определяем реальный тип файла по первым 512 байтам
	buffer := make([]byte, 512) //создаем буфер для чтения первых 512 байт файла
	_, err = file.Read(buffer)
	if err != nil && err != io.EOF { //EOF не ошибка, если файл меньше 512 байт
		return "", fmt.Errorf("Не удалось прочитать первые 512 байт файла: %w", err)
	}
	// Сбрасываем указатель чтения обратно в начало файла!
	_, err = file.Seek(0, io.SeekStart) //возвращаемся в начало файла
	if err != nil {
		return "", fmt.Errorf("Не удалось вернуть указатель файла в начало: %w", err)

	}
	contentType := http.DetectContentType(buffer)
	//Проверяем, разрешен ли этот тип файла
	if !AllowebImageTypes[contentType] {
		return "", fmt.Errorf("Недопустимый тип файла: %s", contentType)
	}
	//Декодируем изображение. Это автоматически отбрасывает большинство метаданных.
	img, detectedFormat, err := image.Decode(file)
	if err != nil {
		// Если image.Decode не справился, файл поврежден или не является изображением
		return "", fmt.Errorf("Не удалось декодировать изображение: %w, формат: %s", err, detectedFormat)
		}
	log.Printf("Изображение успешно декодировано как: %s", detectedFormat)
	//Генерируем уникальное имя файла для хранения
	randomName, err := GenerateSecureToken(16)
	if err != nil {
		return "", fmt.Errorf("Не удалось сгенерировать случайное имя файла: %w", err)
	}
	// Используем расширение, определенное декодером, а не из исходного имени файла
	fileExtension := "." + detectedFormat
	storedFilename = randomName + fileExtension
	filePath := filepath.Join(uploadDir, storedFilename)

	//Создаем новый файл на сервере
	outFile, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("Не удалось создать файл на сервере (%s): %w", filePath, err)
	}
	defer outFile.Close()
	//Перекодируем и сохраняем изображение в новый файл
	switch detectedFormat {
		case "jpeg":
			err = jpeg.Encode(outFile, img, nil)
		case "png":
			err = png.Encode(outFile, img)
		case "gif":
			err = gif.Encode(outFile, img, nil)

		default:
			return "", fmt.Errorf("Неизвестный формат изображения: %s", detectedFormat)	
	}
	// Если кодирование не удалось, удаляем созданный пустой файл
	if err != nil {
		os.Remove(filePath)
		return "", fmt.Errorf("Не удалось закодировать и сохранить изображение: %w", err)
	}

	log.Printf("Изображение успешно закодировано и сохранено как %s", filePath)
	// Возвращаем имя сохраненного файла и nil ошибку
	return storedFilename, nil
}
//определяет Content-Type по расширению для ответа клиенту при просмотре.
func GetImageContentType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
		case ".jpg", ".jpeg":
			return "image/jpeg"
		case ".png":
			return "image/png"
		case ".gif":
			return "image/gif"
     default:
		return "application/octet-stream" // неизвестный тип файла
	}
}