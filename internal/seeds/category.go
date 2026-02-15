package seed

import (
	"errors"

	"gorm.io/gorm"

	"livecommerce/internal/database"
	"livecommerce/internal/models"
)

type CatSeed struct {
	Key      string
	NameFa   string
	ParentKey string 
}

func SeedCategories() error {
	db := database.DB

	seeds := []CatSeed{
		// --- ریشه‌ها ---
		{Key: "electronics", NameFa: "دیجیتال"},
		{Key: "home_kitchen", NameFa: "خانه و آشپزخانه"},
		{Key: "fashion", NameFa: "مد و پوشاک"},
		{Key: "beauty_health", NameFa: "زیبایی و سلامت"},
		{Key: "grocery", NameFa: "سوپرمارکت و خوراکی"},
		{Key: "baby_kids", NameFa: "کودک و نوزاد"},
		{Key: "sports_travel", NameFa: "ورزش و سفر"},
		{Key: "automotive", NameFa: "خودرو و موتورسیکلت"},
		{Key: "books_culture", NameFa: "کتاب و فرهنگ"},

		// --- زیر دسته‌های دیجیتال ---
		{Key: "mobiles", NameFa: "موبایل و لوازم جانبی", ParentKey: "electronics"},
		{Key: "laptops_tablets", NameFa: "لپ‌تاپ و تبلت", ParentKey: "electronics"},
		{Key: "computers_parts", NameFa: "کامپیوتر و قطعات", ParentKey: "electronics"},
		{Key: "audio_video", NameFa: "صوتی و تصویری", ParentKey: "electronics"},
		{Key: "gaming", NameFa: "گیمینگ و کنسول", ParentKey: "electronics"},
		{Key: "cameras", NameFa: "دوربین و تجهیزات", ParentKey: "electronics"},

		// --- خانه و آشپزخانه ---
		{Key: "appliances", NameFa: "لوازم خانگی", ParentKey: "home_kitchen"},
		{Key: "furniture_decor", NameFa: "مبلمان و دکوراسیون", ParentKey: "home_kitchen"},
		{Key: "lighting", NameFa: "روشنایی و لوستر", ParentKey: "home_kitchen"},
		{Key: "tools", NameFa: "ابزار و تجهیزات", ParentKey: "home_kitchen"},

		// --- مد و پوشاک ---
		{Key: "women_clothing", NameFa: "پوشاک زنانه", ParentKey: "fashion"},
		{Key: "men_clothing", NameFa: "پوشاک مردانه", ParentKey: "fashion"},
		{Key: "kids_clothing", NameFa: "پوشاک کودک", ParentKey: "fashion"},
		{Key: "shoes_bags", NameFa: "کفش و کیف", ParentKey: "fashion"},
		{Key: "watches_jewelry", NameFa: "ساعت و زیورآلات", ParentKey: "fashion"},

		// --- زیبایی و سلامت ---
		{Key: "beauty", NameFa: "زیبایی و مراقبت پوست", ParentKey: "beauty_health"},
		{Key: "haircare", NameFa: "مراقبت مو", ParentKey: "beauty_health"},
		{Key: "health", NameFa: "سلامت و تجهیزات پزشکی", ParentKey: "beauty_health"},
		{Key: "perfume", NameFa: "عطر و ادکلن", ParentKey: "beauty_health"},

		// --- سوپرمارکت ---
		{Key: "beverages", NameFa: "نوشیدنی‌ها", ParentKey: "grocery"},
		{Key: "snacks", NameFa: "تنقلات", ParentKey: "grocery"},

		// --- کودک ---
		{Key: "toys", NameFa: "اسباب‌بازی", ParentKey: "baby_kids"},

		// --- ورزش و سفر ---
		{Key: "sports", NameFa: "ورزش و تناسب اندام", ParentKey: "sports_travel"},
		{Key: "outdoor_travel", NameFa: "سفر و کمپینگ", ParentKey: "sports_travel"},

		// --- خودرو ---
		{Key: "car_accessories", NameFa: "لوازم جانبی خودرو", ParentKey: "automotive"},

		// --- کتاب ---
		{Key: "stationery", NameFa: "لوازم تحریر", ParentKey: "books_culture"},
		{Key: "music_movies", NameFa: "موسیقی و فیلم", ParentKey: "books_culture"},
	}

	return db.Transaction(func(tx *gorm.DB) error {
		// 1) اول rootها
		for _, s := range seeds {
			if s.ParentKey != "" {
				continue
			}
			if err := upsertCategory(tx, s.Key, s.NameFa, nil); err != nil {
				return err
			}
		}

		parentIDByKey := map[string]uint{}
		var roots []models.Category
		if err := tx.Select("id", "key").Where("parent_id IS NULL").Find(&roots).Error; err != nil {
			return err
		}
		for _, r := range roots {
			parentIDByKey[r.Key] = r.ID
		}

		for _, s := range seeds {
			if s.ParentKey == "" {
				continue
			}
			pid, ok := parentIDByKey[s.ParentKey]
			if !ok {
				return errors.New("seed parent not found: " + s.ParentKey)
			}
			if err := upsertCategory(tx, s.Key, s.NameFa, &pid); err != nil {
				return err
			}
		}

		return nil
	})
}

func upsertCategory(tx *gorm.DB, key, nameFa string, parentID *uint) error {
	var cat models.Category
	err := tx.Where("key = ?", key).First(&cat).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			cat = models.Category{
				Key:      key,
				NameFa:   nameFa,
				ParentID: parentID,
			}
			return tx.Create(&cat).Error
		}
		return err
	}

	// update اگر چیزی تغییر کرده
	updates := map[string]any{}
	if cat.NameFa != nameFa {
		updates["name_fa"] = nameFa
	}
	// parent تغییر کرده؟
	if (cat.ParentID == nil) != (parentID == nil) {
		updates["parent_id"] = parentID
	} else if cat.ParentID != nil && parentID != nil && *cat.ParentID != *parentID {
		updates["parent_id"] = parentID
	}

	if len(updates) > 0 {
		return tx.Model(&models.Category{}).Where("id = ?", cat.ID).Updates(updates).Error
	}
	return nil
}
