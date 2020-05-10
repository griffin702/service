package validator

import (
	"errors"
	"reflect"
	"strings"
	"sync"

	"github.com/go-playground/locales/zh"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	zhTranslations "github.com/go-playground/validator/v10/translations/zh"
)

type Validator interface {
	ValidateStruct(obj interface{}, translation ...bool) error
	Engine(translation ...bool) interface{}
	RegisterValidation(tag string, fn validator.Func, callValidationEvenIfNull ...bool) error
}

func NewValidator() Validator {
	return &defaultValidator{}
}

type defaultValidator struct {
	once     sync.Once
	validate *validator.Validate
	trans    ut.Translator
}

// ValidateStruct receives any kind of type, but only performed struct or pointer to struct type.
func (v *defaultValidator) ValidateStruct(obj interface{}, translation ...bool) error {
	t := true
	if len(translation) > 0 {
		t = translation[0]
	}
	value := reflect.ValueOf(obj)
	valueType := value.Kind()
	if valueType == reflect.Ptr {
		valueType = value.Elem().Kind()
	}
	if valueType == reflect.Struct {
		v.lazyInit(t)
		if err := v.validate.Struct(obj); err != nil {
			if tErr, ok := err.(validator.ValidationErrors); ok {
				var list []string
				for _, v := range tErr.Translate(v.trans) {
					//list = append(list, fmt.Sprintf("Key: %s Error: %s", k, v))
					list = append(list, v)
				}
				result := strings.Join(list, ", ")
				return errors.New(result)
			}
			return err
		}
	}
	return nil
}

// Engine returns the underlying validator engine which powers the default
// Validator instance. This is useful if you want to register custom validations
// or struct level validations. See validator GoDoc for more info -
// https://godoc.org/gopkg.in/go-playground/validator.v8
func (v *defaultValidator) Engine(translation ...bool) interface{} {
	t := true
	if len(translation) > 0 {
		t = translation[0]
	}
	v.lazyInit(t)
	return v.validate
}

func (v *defaultValidator) lazyInit(translation bool) {
	v.once.Do(func() {
		v.validate = validator.New()
		v.validate.SetTagName("valid")
		_ = v.validate.RegisterValidation("ck_np", checkNewPassword)
		if translation {
			cn := zh.New()
			uni := ut.New(cn, cn)
			v.trans, _ = uni.GetTranslator("zh")
			_ = zhTranslations.RegisterDefaultTranslations(v.validate, v.trans)
		}
	})
}

func (v *defaultValidator) RegisterValidation(tag string, fn validator.Func, callValidationEvenIfNull ...bool) error {
	return v.validate.RegisterValidation(tag, fn, callValidationEvenIfNull...)
}

// 自定义验证函数
func checkNewPassword(fl validator.FieldLevel) bool {
	p := fl.Field()
	if p.Len() > 0 {
		np := fl.Parent().Elem().FieldByName("NewPassword")
		if np.Len() == 0 {
			return false
		}
		npa := fl.Parent().Elem().FieldByName("NewPasswordAgain")
		if npa.Len() == 0 {
			return false
		}
	}
	return true
}
