package auth

import (
	"fmt"
	"golang.org/x/crypto/bcrypt"
)

// HashPassword принимает пароль в виде строки и возвращает его bcrypt-хеш.
// Стоимость (cost) определяет, насколько сложным будет вычисление хеша.
// Используем bcrypt.DefaultCost - это рекомендуемое значение по умолчанию.
func HashPassword(password string) (string, error) {
	// bcrypt.GenerateFromPassword хеширует пароль.
	// []byte(password) преобразует строку в срез байт.
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		// Возвращаем пустую строку и оборачиваем ошибку для контекста
		return "", fmt.Errorf("Ошибка хэширования пароля: %w", err)
	}
	// Преобразуем результат (срез байт) обратно в строку и возвращаем.
	return string(bytes), nil

}

func CheckPasswordHash(password, hash string) bool {
	// bcrypt.CompareHashAndPassword сравнивает хеш (из БД) с паролем (введенным пользователем).
	// Важно: эта функция сама обрабатывает соль, которая встроена в сам bcrypt-хеш.
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	// Если ошибки нет (err == nil), значит пароли совпали.
	return err == nil
}