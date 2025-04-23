package services

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

)

func GenerateSecureToken(length int) (string, error) {
	// Создаем срез байт нужной длины
	b := make([]byte, length)
	// Читаем случайные байты из криптографического источника ОС
	_, err := rand.Read(b)
	if err != nil {
		// Если ОС не может предоставить случайные данные, это серьезная проблема
		return "", fmt.Errorf("Не удалось сгенерировать случайные байты: %w", err)
	}
	// Кодируем байты в URL-safe base64 строку (без символов '+' и '/')
	// RawURLEncoding убирает еще и padding '=' в конце, делая строку чище для URL.
	return base64.URLEncoding.EncodeToString(b), nil
}
