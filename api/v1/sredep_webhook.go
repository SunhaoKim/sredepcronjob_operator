/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	cron "github.com/robfig/cron/v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	validationutils "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var sredeplog = logf.Log.WithName("sredep-resource")

//我们将 webhook 和 manager 关联起来。
func (r *Sredep) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-app-operator-com-v1-sredep,mutating=true,failurePolicy=fail,sideEffects=None,groups=app.operator.com,resources=sredeps,verbs=create;update,versions=v1,name=msredep.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Defaulter = &Sredep{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
// Default 实现了 webhook.Defaulter ，因此将为该类型注册一个webhook。
func (r *Sredep) Default() {
	sredeplog.Info("default", "name", r.Name)
	if r.Spec.ConcurrencyPolicy == "" {
		r.Spec.ConcurrencyPolicy = AllowConcurrent
	}
	if r.Spec.Suspend == nil {
		r.Spec.Suspend = new(bool)
	}
	if r.Spec.SuccessfulJobsHistoryLimit == nil {
		r.Spec.SuccessfulJobsHistoryLimit = new(int32)
		*r.Spec.SuccessfulJobsHistoryLimit = 3
	}
	if r.Spec.FailedJobsHistoryLimit == nil {
		r.Spec.FailedJobsHistoryLimit = new(int32)
		*r.Spec.FailedJobsHistoryLimit = 1
	}
	// TODO(user): fill in your defaulting logic.
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-app-operator-com-v1-sredep,mutating=false,failurePolicy=fail,sideEffects=None,groups=app.operator.com,resources=sredeps,verbs=create;update;delete,versions=v1,name=vsredep.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Validator = &Sredep{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Sredep) ValidateCreate() error {
	sredeplog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return r.validateSredep()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Sredep) ValidateUpdate(old runtime.Object) error {
	sredeplog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return r.validateSredep()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Sredep) ValidateDelete() error {
	sredeplog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return r.validateSredep()
}

func (r *Sredep) validateSredep() error {
	var allerrs field.ErrorList
	if err := r.validateSredepName(); err != nil {
		allerrs = append(allerrs, err)
	}
	if err := r.validateSredepSpec(); err != nil {
		allerrs = append(allerrs, err)
	}
	if len(allerrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: "app.operator.com", Kind: "Sredep"},
		r.Name, allerrs)
}

func (r *Sredep) validateSredepSpec() *field.Error {
	// kubernetes API machinery 的字段助手会帮助我们很好地返回结构化的验证错误。
	return validateScheduleFormat(
		r.Spec.Schedule,
		field.NewPath("spec").Child("schedule"))
}

func validateScheduleFormat(schedule string, fldPath *field.Path) *field.Error {
	if _, err := cron.ParseStandard(schedule); err != nil {
		return field.Invalid(fldPath, schedule, err.Error())
	}
	return nil
}

func (r *Sredep) validateSredepName() *field.Error {
	if len(r.ObjectMeta.Name) > validationutils.DNS1035LabelMaxLength-11 {
		// job 的名字长度像所有 Kubernetes 对象一样是是 63 字符(必须适合 DNS 子域)。
		// 在创建 job 的时候，cronjob 的控制器会添加一个 11 字符的后缀(`-$TIMESTAMP`)。
		// job 的名字长度限制在 63 字符。因此 cronjob 的名字的长度一定小于等于 63-11=52 。
		// 如果这里我们没有进行验证，后面当job创建的时候就会失败。
		return field.Invalid(field.NewPath("metadata").Child("name"), r.Name, "must be no more than 52 characters")
	}
	return nil
}
