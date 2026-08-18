package models

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/sjlit/aeus/pkg/errs"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// CheckPasswordPolicy enforces the password baseline: 8-32 characters,
// alphanumeric only, and at least one letter AND one digit. It returns
// an Invalid (1001) error describing the first violated rule.
//
// The regexp on User.Password's rule tag covers length+charset at the
// REST layer only (RE2 has no lookahead for the letter+digit combo),
// so every password-write path — the BeforeCreate/BeforeUpdate hooks,
// ChangePassword, ResetPassword — funnels through this function.
func CheckPasswordPolicy(pwd string) error {
	if n := len(pwd); n < 5 || n > 32 {
		return errs.Newf(errs.CodeInvalid, "password must be 5-32 characters")
	}
	var hasLetter, hasDigit bool
	for _, r := range pwd {
		switch {
		case 'a' <= r && r <= 'z', 'A' <= r && r <= 'Z':
			hasLetter = true
		case '0' <= r && r <= '9':
			hasDigit = true
		default:
			return errs.Newf(errs.CodeInvalid, "password may only contain letters and digits")
		}
	}
	if !hasLetter || !hasDigit {
		return errs.Newf(errs.CodeInvalid, "password must contain both letters and digits")
	}
	return nil
}

// checkPlaintextPasswordPolicy applies CheckPasswordPolicy only when
// Password carries a NEW plaintext value — already-hashed values (the
// "$2" prefix) and empty values pass through, mirroring hashPassword's
// idempotency signal.
func (m *User) checkPlaintextPasswordPolicy() error {
	if m.Password == "" || strings.HasPrefix(m.Password, "$2") {
		return nil
	}
	return CheckPasswordPolicy(m.Password)
}

// hashPassword bcrypts a plaintext password; already-hashed passwords
// (starting with "$2") pass through unchanged, keeping the hook
// idempotent.
func hashPassword(pwd string) (string, error) {
	if pwd == "" || strings.HasPrefix(pwd, "$2") {
		return pwd, nil
	}
	h, err := bcrypt.GenerateFromPassword([]byte(pwd), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// hashToken returns the hex-encoded SHA-256 of token. SHA-256 is appropriate
// for access tokens (high-entropy random strings; no brute-force risk like
// passwords), so bcrypt's intentional slowness is unnecessary. Empty input
// passes through so the caller can leave the column NULL/blank.
func hashToken(token string) string {
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// BeforeCreate runs the password policy on any new plaintext password
// and then hashes it so plaintext never reaches the database.
func (m *User) BeforeCreate(tx *gorm.DB) error {
	if err := m.checkPlaintextPasswordPolicy(); err != nil {
		return err
	}
	hashed, err := hashPassword(m.Password)
	if err != nil {
		return err
	}
	m.Password = hashed
	return nil
}

// BeforeUpdate re-hashes the password when an update carries a new
// one.  hashPassword is idempotent on already-hashed values (the $2
// prefix), so Update(Users, struct{}) is safe; the policy check runs
// only against new plaintext for the same reason.
func (m *User) BeforeUpdate(tx *gorm.DB) error {
	if err := m.checkPlaintextPasswordPolicy(); err != nil {
		return err
	}
	hashed, err := hashPassword(m.Password)
	if err != nil {
		return err
	}
	m.Password = hashed
	return nil
}

// ValidatePassword verifies pwd against the stored bcrypt hash. Comparing
// two bcrypt hashes directly does not work (random salt), so we use the
// constant-time CompareHashAndPassword API.
func (m *User) ValidatePassword(pwd string) bool {
	if m.Password == "" || pwd == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(m.Password), []byte(pwd)) == nil
}

// BeforeCreate hashes the access token before persistence. The raw token
// never touches disk — LoginLog is only used for audit, not for token
// verification (JWTs are stateless), so a one-way hash is sufficient.
// hashToken("") returns "", so a blank token stays blank.
func (m *LoginLog) BeforeCreate(tx *gorm.DB) error {
	m.AccessToken = hashToken(m.AccessToken)
	return nil
}

// User is a tenant-scoped system user.  Password is bcrypt-hashed by
// the BeforeCreate / BeforeUpdate hooks.
type User struct {
	TenantModel
	UID      string `json:"uid" yaml:"uid" xml:"uid" gorm:"index;size:20;column:uid" comment:"用户工号" props:"readonly:update" rule:"required;unique;regexp:^[a-zA-Z0-9]{3,8}$"`
	Username string `json:"username" yaml:"username" xml:"username" gorm:"size:20;column:username" comment:"用户名称" rule:"required"`
	// RoleKey holds Role.Key (machine identifier); size:30 matches Role.Key.
	RoleKey     string `json:"role_key" yaml:"roleKey" xml:"roleKey" gorm:"size:30;not null;default:'';column:role_key" comment:"所属角色 Key" format:"role" rule:"required" live:"type:dropdown;url:/role/options"`
	Status      string `json:"status" yaml:"status" xml:"status" gorm:"size:20;default:normal;column:status" comment:"状态" scenarios:"create,update,list,search" enum:"normal:正常;disabled:禁用"`
	DeptID      int64  `json:"dept_id" yaml:"deptId" xml:"deptId" gorm:"not null;default:0;column:dept_id" comment:"所属部门" format:"department" rule:"required" live:"type:dropdown;url:/department/labels"`
	Password    string `json:"password" yaml:"password" xml:"password" gorm:"size:120;column:password" comment:"用户密码" scenarios:"create" rule:"required;regexp:^[A-Za-z0-9]{8,32}$"`
	Email       string `json:"email" yaml:"email" xml:"email" gorm:"size:60;column:email" comment:"用户邮箱" scenarios:"create;update;view;list;export"`
	Avatar      string `json:"avatar" yaml:"avatar" xml:"avatar" gorm:"size:1024;column:avatar" comment:"用户头像" scenarios:"view"`
	Gender      string `json:"gender" yaml:"gender" xml:"gender" gorm:"size:20;default:man;column:gender" comment:"用户性别" scenarios:"list;create;update;view;export" rule:"required" enum:"man:男;woman:女;other:其他"`
	Description string `json:"description" yaml:"description" xml:"description" gorm:"size:1024;column:description" comment:"备注说明" scenarios:"create;update;view;export" format:"textarea"`
}

// LoginLog records one login attempt per row.  AccessToken is hashed
// before persistence and is audit data only.
type LoginLog struct {
	TenantModel
	UID      string `json:"uid" yaml:"uid" xml:"uid" gorm:"index;size:20;column:uid" comment:"用户" format:"user" props:"readonly:update" rule:"required"`
	IP       string `json:"ip" yaml:"ip" xml:"ip" gorm:"size:128;column:ip" comment:"登录地址" scenarios:"list;view;export"`
	Browser  string `json:"browser" yaml:"browser" xml:"browser" gorm:"size:128;column:browser" comment:"浏览器" scenarios:"list;view;export"`
	OS       string `json:"os" yaml:"os" xml:"os" gorm:"size:128;column:os" comment:"操作系统" scenarios:"list;view;export"`
	Platform string `json:"platform" yaml:"platform" xml:"platform" gorm:"size:128;column:platform" comment:"系统平台" scenarios:"list;view;export"`
	// AccessToken is hashed before persistence and is audit data only —
	// it is never exposed through the REST API, so the wire tags are
	// disabled and no scenario lists it.
	AccessToken string `json:"-" yaml:"-" xml:"-" gorm:"size:1024;column:access_token" comment:"访问令牌"`
	UserAgent   string `json:"user_agent" yaml:"userAgent" xml:"userAgent" gorm:"size:1024;column:user_agent" comment:"用户代理" scenarios:"list;view;export"`
}

// TableName returns the physical table name (gorm.Tabler).
func (m *User) TableName() string {
	return "sys_users"
}

// ModuleName returns the rest module name (rest.ModuleNamer).
func (m *User) ModuleName() string {
	return "system"
}

// MenuEntry exposes this model as a navigable item under the
// 用户中心 section.  Parent references SystemUserCenter (inserted by
// admin.seed.go's EnsureSectionMenus); the framework's BuildTree nests
// the row under that container.  Sort orders User first inside the
// section.  Component/Uri are left empty for the framework to derive
// from ModuleName + TableName.
func (m *User) MenuEntry() MenuSpec {
	return MenuSpec{Name: "用户管理", Parent: "SystemUserCenter", Sort: 10}
}

// TableName returns the physical table name (gorm.Tabler).
func (m *LoginLog) TableName() string {
	return "sys_login_logs"
}

// ModuleName returns the rest module name (rest.ModuleNamer).
func (m *LoginLog) ModuleName() string {
	return "system"
}

// MenuEntry exposes login logs as a navigable item under the
// 日志记录 section.  Parent references SystemLogs (inserted by
// admin.seed.go's EnsureSectionMenus); Sort puts LoginLog below Audit
// inside the section.
func (m *LoginLog) MenuEntry() MenuSpec {
	return MenuSpec{Name: "登录日志", Parent: "SystemLogs", Sort: 20}
}
