package handlers

import (
	"OpsLB/database"
	"OpsLB/models"
	"github.com/gin-gonic/gin"
	"net/http"
)

// 获取系统配置
func GetSystemConfig(c *gin.Context) {
	var configs []models.SystemConfig
	if err := database.DB.Find(&configs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	// 转换为 map
	result := make(map[string]string)
	for _, config := range configs {
		result[config.Key] = config.Value
	}
	
	c.JSON(http.StatusOK, result)
}

// 更新系统配置
func UpdateSystemConfig(c *gin.Context) {
	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	for key, value := range req {
		var config models.SystemConfig
		result := database.DB.Where("config_key = ?", key).First(&config)
		
		if result.Error != nil {
			// 创建新配置
			config = models.SystemConfig{
				Key:   key,
				Value: value,
			}
			database.DB.Create(&config)
		} else {
			// 更新配置
			database.DB.Model(&config).Update("config_value", value)
		}
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "Config updated successfully"})
}
