// 控制台密码的哈希和校验。明文密码不入库。
package store

import "golang.org/x/crypto/bcrypt"

// 生成密码哈希。失败时不要把明文存进用户表。
func HashPassword(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// 校验密码。哈希格式不对时返回 false，不返回错误。
func CheckPassword(hash, plain string) bool {
	if hash == "" || plain == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
