package services

import (
	"fmt"
	"keizer-auth/internal/models"
	"keizer-auth/internal/repositories"
	"keizer-auth/internal/utils"
	"keizer-auth/internal/validators"
	"time"

	"github.com/nrednav/cuid2"
	"github.com/redis/go-redis/v9"
)

type AuthService struct {
	userRepo  *repositories.UserRepository
	redisRepo *repositories.RedisRepository
}

func NewAuthService(userRepo *repositories.UserRepository, redisRepo *repositories.RedisRepository) *AuthService {
	return &AuthService{userRepo: userRepo, redisRepo: redisRepo}
}

func (as *AuthService) RegisterUser(userRegister *validators.SignUpUser) (string, error) {
	passwordHash, err := utils.HashPassword(userRegister.Password)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}

	otp, err := utils.GenerateOTP()
	if err != nil {
		return "", fmt.Errorf("failed to generate OTP: %w", err)
	}

	id := cuid2.Generate()
	hashOtp, err := utils.HashPassword(otp)
	if err != nil {
		return "", fmt.Errorf("failed to hash OTP: %w", err)
	}

	otpData := map[string]interface{}{
		"otp_hash": hashOtp,
		"email":    userRegister.Email,
	}

	err = as.redisRepo.HSet(id, otpData, time.Minute*3)
	if err != nil {
		return "", fmt.Errorf("failed to save otp in redis: %w", err)
	}

	err = as.redisRepo.Expire(id, time.Minute*3)
	if err != nil {
		return "", fmt.Errorf("failed to set expiration for otp in redis: %w", err)
	}

	// TODO: track status, add reties
	go SendOTPEmail(userRegister.Email, otp)

	if err = as.userRepo.CreateUser(&models.User{
		Email:        userRegister.Email,
		FirstName:    userRegister.FirstName,
		LastName:     userRegister.LastName,
		PasswordHash: passwordHash,
	}); err != nil {
		return "", err
	}

	return id, nil
}

func (as *AuthService) VerifyOTP(verifyOtpBody *validators.VerifyOTP) (bool, error) {
	otpData, err := as.redisRepo.HGetAll(verifyOtpBody.Id)
	if err != nil {
		if err == redis.Nil {
			return false, fmt.Errorf("otp expired")
		}
		return false, fmt.Errorf("failed to get otp from redis %w", err)
	}

	return utils.VerifyPassword(verifyOtpBody.Otp, otpData["otp_hash"])
}
