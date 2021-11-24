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

package controllers

import (
	"context"
	"fmt"
	"sort"
	"time"

	appv1 "github.com/SunhaoKim/deployment_operator/api/v1"
	"github.com/go-logr/logr"
	cron "github.com/robfig/cron/v3"
	kbatch "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ref "k8s.io/client-go/tools/reference"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SredepReconciler reconciles a Sredep object
//定义结构体
type SredepReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
	Clock
}

//定义变量 sredep既为cronjob，定义成了上层对象，其余变量为jobs各种类型变量
var (
	sredep                  appv1.Sredep
	sredepJobs              kbatch.JobList
	sreactiveJobs           []*kbatch.Job
	sresuccessfulJobs       []*kbatch.Job
	srefailedJobs           []*kbatch.Job
	mostRecentTime          *time.Time
	apiGVStr                = appv1.GroupVersion.String()
	jobOwnerKey             = ".metadata.controller"
	scheduledTimeAnnotation = "app.operator.com/scheduled-at"
)

//定义虚拟时钟方便调节时间
type realClock struct{}

//定义NOW方法 返回当前真实时间
func (realClock) Now() time.Time { return time.Now() }

//定义clock接口 获取当前时间
type Clock interface {
	Now() time.Time
}

//添加rbac字段 修改job权限
//+kubebuilder:rbac:groups=app.operator.com,resources=sredeps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=app.operator.com,resources=sredeps/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=app.operator.com,resources=sredeps/finalizers,verbs=update
//+kubebuilder:rbac:groups=app,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=app,resources=jobs/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Sredep object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
//索引作用 调谐器会获取 cronjob 下的所有 job 以更新它们状态。随着 cronjob 数量的增加，遍历全部 conjob 查找会变的相当低效。为了提高查询效率，这些任务会根据控制器名称建立索引。缓存后的 job 对象会 被添加上一个 jobOwnerKey 字段。这个字段引用其归属控制器和函数作为索引。在下文中，我们将配置 manager 作为这个字段的索引
func (r *SredepReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = r.Log.WithValues("srecronjob", req.NamespacedName)
	//获取cronjob定时任务 1. 根据名称加载定时任务
	if err := r.Get(ctx, req.NamespacedName, &sredep); err != nil {
		r.Log.Error(err, "unable to fetch CronJob")
		//忽略掉 not-found 错误，它们不能通过重新排队修复（要等待新的通知）
		//在删除一个不存在的对象时，可能会报这个错误。
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	//2.列出有效的Job 更新它们的状态,注意，我们使用变长参数 来映射命名空间和任意多个匹配变量（实际上相当于是建立了一个索引
	if err := r.List(ctx, &sredepJobs, client.InNamespace(req.Namespace), client.MatchingFields{jobOwnerKey: req.Name}); err != nil {
		r.Log.Error(err, "unable to list sre jobs")
		return ctrl.Result{}, err
	}
	// 当一个 job 被标记为 “succeeded” 或 “failed” 时，我们认为这个任务处于 “finished” 状态。 Status conditions 允许我们给 job 对象添加额外的状态信息，开发人员或控制器可以通过 这些校验信息来检查 job 的完成或健康状态。
	isJobFinished := func(job *kbatch.Job) (bool, kbatch.JobConditionType) {
		for _, c := range job.Status.Conditions {
			if (c.Type == kbatch.JobComplete || c.Type == kbatch.JobFailed) && c.Status == corev1.ConditionTrue {
				return true, c.Type
			}
		}
		return false, ""
	}
	//使用辅助函数来提取创建 job 时注释中排定的执行时间
	getScheduledTimeForJob := func(job *kbatch.Job) (*time.Time, error) {
		timeRaw := job.Annotations[scheduledTimeAnnotation]
		if len(timeRaw) == 0 {
			return nil, nil
		}

		timeParsed, err := time.Parse(time.RFC3339, timeRaw)
		if err != nil {
			return nil, err
		}
		return &timeParsed, nil
	}
	for i, job := range sredepJobs.Items {
		_, finishedType := isJobFinished(&job)
		//判断job类别
		switch finishedType {
		case "":
			sreactiveJobs = append(sreactiveJobs, &sredepJobs.Items[i])
		case kbatch.JobFailed:
			srefailedJobs = append(srefailedJobs, &sredepJobs.Items[i])
		case kbatch.JobComplete:
			sresuccessfulJobs = append(sresuccessfulJobs, &sredepJobs.Items[i])
		}
		//将启动时间放进注释中，当job生效从中读取
		scheduledTimeForJob, err := getScheduledTimeForJob(&job)
		if err != nil {
			r.Log.Error(err, "unable to parse schedule time for job", "job", &job)
			continue
		}
		if scheduledTimeForJob != nil {
			if mostRecentTime == nil {
				mostRecentTime = scheduledTimeForJob
			} else if mostRecentTime.Before(*scheduledTimeForJob) {
				mostRecentTime = scheduledTimeForJob
			}
		}
	}
	if mostRecentTime != nil {
		sredep.Status.LastScheduleTime = &metav1.Time{Time: *mostRecentTime}
	} else {
		sredep.Status.LastScheduleTime = nil
	}
	sredep.Status.Active = nil
	for _, sreactiveJob := range sreactiveJobs {
		jobRef, err := ref.GetReference(r.Scheme, sreactiveJob)
		if err != nil {
			r.Log.Error(err, "unable to make reference to active job", "job", sreactiveJob)
			continue
		}
		sredep.Status.Active = append(sredep.Status.Active, *jobRef)
	}
	r.Log.V(1).Info("job count", "active jobs", len(sreactiveJobs), "successful jobs", len(sresuccessfulJobs), "failed jobs", len(srefailedJobs))
	if err := r.Status().Update(ctx, &sredep); err != nil {
		r.Log.Error(err, "unable to update sredep status")
		return ctrl.Result{}, err
	}
	// 3.根据保留的历史版本清理过旧的job
	// 注意: 删除操作采用的“尽力而为”策略
	// 如果个别 job 删除失败了，不会将其重新排队，直接结束删除操作
	if sredep.Spec.FailedJobsHistoryLimit != nil {
		sort.Slice(srefailedJobs, func(i, j int) bool {
			if sresuccessfulJobs[i].Status.StartTime == nil {
				return srefailedJobs[j].Status.StartTime != nil
			}
			return srefailedJobs[i].Status.StartTime.Before(srefailedJobs[j].Status.StartTime)
		})
		for i, job := range srefailedJobs {
			if int32(i) >= int32(len(srefailedJobs))-*sredep.Spec.FailedJobsHistoryLimit {
				break
			}
			if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); client.IgnoreNotFound(err) != nil {
				r.Log.Error(err, "unable to delete old failed job", "job", job)
			} else {
				r.Log.V(0).Info("delete old failed job", "job", job)
			}
		}
	}
	//4 检查是否被挂起
	if sredep.Spec.Suspend != nil && *sredep.Spec.Suspend {
		r.Log.V(1).Info("cronjob suspended,skipping")
		return ctrl.Result{}, nil
	}
	// your logic here
	//5.借助cron库获取下一次job执行时间
	getNextSchedule := func(sredep *appv1.Sredep, now time.Time) (lastMissed time.Time, next time.Time, err error) {
		sched, err := cron.ParseStandard(sredep.Spec.Schedule)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("unparseable schedule %q: %v", sredep.Spec.Schedule, err)
		}
		// 出于优化的目的，我们可以使用点技巧。从上一次观察到的执行时间开始执行，
		// 这个执行时间可以被在这里被读取。但是意义不大，因为我们刚更新了这个值。
		var earliestTime time.Time
		//定义最早执行时间，如果没有获取到 选job创建时间
		if sredep.Status.LastScheduleTime != nil {
			earliestTime = sredep.Status.LastScheduleTime.Time
		} else {
			earliestTime = sredep.ObjectMeta.CreationTimestamp.Time
		}
		if sredep.Spec.StartingDeadlineSeconds != nil {
			//如果开始时间 超过截止时间，不再执行
			scheduledeadline := now.Add(-time.Second * time.Duration(*sredep.Spec.StartingDeadlineSeconds))
			if scheduledeadline.After(earliestTime) {
				earliestTime = scheduledeadline
			}
		}
		if earliestTime.After(now) {
			return time.Time{}, sched.Next(now), nil
		}
		starts := 0
		for t := sched.Next(earliestTime); !t.After(now); t = sched.Next(t) {
			lastMissed = t
			starts++
			if starts > 80 {
				// 获取不到最近一次执行时间，直接返回空切片
				return time.Time{}, time.Time{}, fmt.Errorf("too many missed start times (> 80). Set or decrease .spec.startingDeadlineSeconds or check clock skew")
			}
		}
		return lastMissed, sched.Next(now), nil
	}
	// 计算出定时任务下一次执行时间（或是遗漏的执行时间）
	missedRun, nextRun, err := getNextSchedule(&sredep, r.Now())
	if err != nil {
		r.Log.Error(err, "unable to figure out sredep scheduler")
		return ctrl.Result{}, nil
	}
	//上述步骤执行完后，将准备好的请求加入队列直到下次执行， 然后确定这些 job 是否要实际执行
	scheduledResult := ctrl.Result{RequeueAfter: nextRun.Sub(r.Now())}
	r.Log.WithValues("now", r.Now(), "next run", nextRun)
	//6.如果 job 符合执行时机，并且没有超出截止时间，且不被并发策略阻塞，执行该 job 如果 job 遗漏了一次执行，且还没超出截止时间，把遗漏的这次执行也补上
	if missedRun.IsZero() {
		r.Log.V(1).Info("no upcoming scheduled times, sleeping until next")
		return scheduledResult, nil
	}
	// 确保错过的执行没有超过截止时间
	r.Log.WithValues("current run", missedRun)
	tooLate := false
	if sredep.Spec.StartingDeadlineSeconds != nil {
		tooLate = missedRun.Add(time.Duration(*sredep.Spec.StartingDeadlineSeconds) * time.Second).Before(r.Now())
	}
	if tooLate {
		r.Log.V(1).Info("missed starting deadline for last run, sleeping till next")
		return scheduledResult, nil
	}
	//如果确认 job 需要实际执行。我们有三种策略执行该 job。要么先等待现有的 job 执行完后，在启动本次 job； 或是直接覆盖取代现有的 job；或是不考虑现有的 job，直接作为新的 job 执行。因为缓存导致的信息有所延迟， 当更新信息后需要重新排队。
	// 确定要 job 的执行策略 —— 并发策略可能禁止多个job同时运行
	if sredep.Spec.ConcurrencyPolicy == appv1.ForbidConcurrent && len(sreactiveJobs) > 0 {
		r.Log.V(1).Info("concurrency policy blocks concurrent runs, skipping", "num active", len(sreactiveJobs))
		return scheduledResult, nil
	}
	// 直接覆盖现有 job
	if sredep.Spec.ConcurrencyPolicy == appv1.ReplaceConcurrent {
		for _, activeJob := range sreactiveJobs {
			// we don't care if the job was already deleted
			if err := r.Delete(ctx, activeJob, client.PropagationPolicy(metav1.DeletePropagationBackground)); client.IgnoreNotFound(err) != nil {
				r.Log.Error(err, "unable to delete active job", "job", activeJob)
				return ctrl.Result{}, err
			}
		}
	}
	//基于 CronJob 模版构建 job，从模板复制 spec 及对象的元信息。
	//然后在注解中设置执行时间，这样我们可以在每次的调谐中获取起作为“上一次执行时间”
	//最后，还需要设置 owner reference字段。当我们删除 CronJob 时，Kubernetes 垃圾收集 器会根据这个字段对应的 job。同时，当某个job状态发生变更（创建，删除，完成）时， controller-runtime 可以根据这个字段识别出要对那个 CronJob 进行调谐。
	constructJobForCronJob := func(sredep *appv1.Sredep, scheduledtime time.Time) (*kbatch.Job, error) {
		// job 名称带上执行时间以确保唯一性，避免排定执行时间的 job 创建两次
		name := fmt.Sprintf("%s-%d", sredep.Name, scheduledtime.Unix())
		job := &kbatch.Job{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      make(map[string]string),
				Annotations: map[string]string{},
				Name:        name,
				Namespace:   sredep.Namespace,
			},
			Spec: *sredep.Spec.JobTemplate.Spec.DeepCopy(),
		}
		for k, v := range sredep.Spec.JobTemplate.Annotations {
			job.Annotations[k] = v
		}
		job.Annotations[scheduledTimeAnnotation] = scheduledtime.Format(time.RFC3339)
		for k, v := range sredep.Spec.JobTemplate.Labels {
			job.Labels[k] = v
		}
		if err := ctrl.SetControllerReference(sredep, job, r.Scheme); err != nil {
			return nil, err
		}
		return job, nil
	}
	//构建job
	job, err := constructJobForCronJob(&sredep, missedRun)
	if err != nil {
		r.Log.Error(err, "unable to construct job from template")
		// job 的 spec 没有变更，无需重新排队
		return scheduledResult, err
	}
	//在集群中创建job
	if err := r.Create(ctx, job); err != nil {
		r.Log.Error(err, "unable to create job for sredep", "job", job)
		return ctrl.Result{}, err
	}
	r.Log.V(1).Info("created Job for CronJob run", "job", job)
	//7: 当 job 开始运行或到了 job 下一次的执行时间，重新排队
	//最终我们返回上述预备的结果。我们还需重新排队当任务还有下一次执行时。 这被视作最长截止时间——如果期间发生了变更，例如 job 被提前启动或是提前 结束，或被修改，我们可能会更早进行调谐。
	return scheduledResult, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SredepReconciler) SetupWithManager(mgr ctrl.Manager) error {
	//此处创建一个真实的时钟
	if r.Clock == nil {
		r.Clock = realClock{}
	}
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &kbatch.Job{}, jobOwnerKey, func(rawObj client.Object) []string {
		//获取 job 对象，提取 owner...
		job := rawObj.(*kbatch.Job)
		owner := metav1.GetControllerOf(job)
		if owner == nil {
			return nil
		}
		// ...确保 owner 是个 CronJob...
		if owner.APIVersion != apiGVStr || owner.Kind != "CronJob" {
			return nil
		}

		// ...是 CronJob，返回
		return []string{owner.Name}
	}); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&appv1.Sredep{}).
		Complete(r)
}
