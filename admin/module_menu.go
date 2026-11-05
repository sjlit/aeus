package admin

import (
	"errors"
	"fmt"

	"github.com/sjlit/aeus/admin/models"
	"gorm.io/gorm"
)

// EnsureModuleMenu FirstOrCreates the top-level menu section for a
// business module generated from (aeus.module). It is idempotent and
// never overwrites an operator's edits.
func EnsureModuleMenu(db *gorm.DB, spec models.ModuleSpec) error {
	if db == nil {
		return errors.New("admin: EnsureModuleMenu requires a non-nil *gorm.DB")
	}
	if spec.Name == "" {
		return fmt.Errorf("admin: module name is required")
	}
	if spec.Title == "" {
		return fmt.Errorf("admin: module title is required")
	}
	component := spec.Component
	if component == "" {
		component = pascalWords(spec.Name) + "Center"
	}
	var existing models.Menu
	err := db.Unscoped().Where("component = ?", component).First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("lookup module menu %q: %w", component, err)
	}
	row := models.Menu{
		Component: component,
		Name:      spec.Title,
		Icon:      spec.Icon,
		Sort:      spec.Sort,
	}
	if err := db.Create(&row).Error; err != nil {
		return fmt.Errorf("insert module menu %q: %w", component, err)
	}
	return nil
}
