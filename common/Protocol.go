package common

import (
	"context"
	"encoding/json"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/gorhill/cronexpr"
)

//Worker节点SSH信息
type WorkerSSH struct {
	User string `json:"user"` //用户名
	Pwd  string `json:"pwd"`  //密码
	Addr string `json:"addr"` //IP地址
}

//定时任务
type Job struct {
	Name      string `json:"name"`      //任务名
	Command   string `json:"command"`   //shell命令
	CronExpr  string `json:"cronExpr"`  //cron表达式
	Email     string `json:"email"`     //报警邮件
	StartTime string `json:"startTime"` //任务开始时间
	StopTime  string `json:"stopTime"`  //任务停止时间
	Details   string `json:"details"`   //任务详情
}

//任务调度计划
type JobSchedulePlan struct {
	Job      *Job                 //要调度的任务信息
	Expr     *cronexpr.Expression //解析好的cronexpr表达式
	NextTime time.Time            //下次调度时间
}

//任务执行状态
type JobExecuteInfo struct {
	Job        *Job               //任务信息
	PlanTime   time.Time          //理论上的调度时间
	RealTime   time.Time          //实际的调度时间
	CancelCtx  context.Context    //任务command的context
	CancelFunc context.CancelFunc //用于取消command执行的cancel函数
}

//http接口应答
type Response struct {
	Errno int         `json:"errno"`
	Msg   string      `json:"msg"`
	Data  interface{} `json:"data"`
}

//变化事件
type JobEvent struct {
	EventType int //SAVE, DELETE
	Job       *Job
}

//任务执行结果
type JobExecuteResult struct {
	ExecuteInfo *JobExecuteInfo //执行状态
	Output      []byte          //脚本输出
	Err         error           //脚本错误原因
	StartTime   time.Time       //启动时间
	EndTime     time.Time       //结束时间
}

//任务执行日志
type JobLog struct {
	JobName      string `json:"jobName" bson:"jobName"`           //任务名字
	Command      string `json:"command" bson:"command"`           //脚本命令
	Err          string `json:"err" bson:"err"`                   //错误原因
	Output       string `json:"output" bson:"output"`             //脚本输出
	PlanTime     int64  `json:"planTime" bson:"planTime"`         //计划开始时间
	ScheduleTime int64  `json:"scheduleTime" bson:"scheduleTime"` //实际调度时间
	StartTime    int64  `json:"startTime" bson:"startTime"`       //任务执行开始时间
	EndTime      int64  `json:"endTime" bson:"endTime"`           //任务执行结束时间
	LocalIP      string `json:"localIP" bson:"localIP"`           //工作Worker节点IP
	Email        string `json:"email" bson:"email"`               //报警邮箱
}

//日志批次
type LogBatch struct {
	Logs []interface{} //多条日志
}

//任务日志过滤条件
type JobLogFilter struct {
	JobName string `bson:"jobName"`
}

//任务日志排序规则
type SortLogByStartTime struct {
	SortOrder int `bson:"startTime"` //{startTime: -1}
}

//应答方法
func BuildResponse(errno int, msg string, data interface{}) (resp []byte, err error) {
	//1.定义一个response
	var (
		response Response
	)

	response.Errno = errno
	response.Msg = msg
	response.Data = data

	//2.序列化json
	resp, err = json.Marshal(response)
	return
}

//反序列化Job
func UnpackJob(value []byte) (ret *Job, err error) {
	var (
		job *Job
	)

	job = &Job{}
	if err = json.Unmarshal(value, job); err != nil {
		return
	}
	ret = job
	return
}

//从etcd的key中提取任务名
///cron/jobs/job10抹掉/cron/jobs/
func ExtractJobName(jobKey string) string {
	return strings.TrimPrefix(jobKey, JOB_SAVE_DIR)
}

//从/cron/killer/job10提取job10
func ExtractKillerName(killerKey string) string {
	return strings.TrimPrefix(killerKey, JOB_KILLER_DIR)
}

//从/cron/once/job1提取job1
func ExtractOnceName(OnceKey string) string {
	return strings.TrimPrefix(OnceKey, JOB_ONCE_DIR)
}

//任务变化事件有2种：1）更新任务 2）删除任务
func BuildJobEvent(eventType int, job *Job) (jobEvent *JobEvent) {
	return &JobEvent{
		EventType: eventType,
		Job:       job,
	}
}

//构造任务执行计划
func BuildJobSchedulePlan(job *Job) (jobSchedulePlan *JobSchedulePlan, err error) {
	var (
		expr *cronexpr.Expression
	)

	//解析JOB的cron表达式
	if expr, err = cronexpr.Parse(job.CronExpr); err != nil {
		return
	}

	//生成任务调度计划对象
	jobSchedulePlan = &JobSchedulePlan{
		Job:      job,
		Expr:     expr,
		NextTime: expr.Next(time.Now()),
	}
	return
}

//构造执行状态信息
func BuildJobExecuteInfo(jobSchedulePlan *JobSchedulePlan) (jobExecuteInfo *JobExecuteInfo) {
	jobExecuteInfo = &JobExecuteInfo{
		Job:      jobSchedulePlan.Job,
		PlanTime: jobSchedulePlan.NextTime, //计算调度时间
		RealTime: time.Now(),               //真实调度时间
	}
	jobExecuteInfo.CancelCtx, jobExecuteInfo.CancelFunc = context.WithCancel(context.TODO())
	return
}

//提取worker的IP
func ExtractWorkerIP(regKey string) string {
	return strings.TrimPrefix(regKey, JOB_WORKER_DIR)
}

// slice去重
func StrSliceRemoveRepeat(slice []string) (newSlice []string) {
	for _, val := range slice {
		if len(newSlice) == 0 {
			newSlice = append(newSlice, val)
		} else {
			for k, v := range newSlice {
				if val == v {
					break
				}
				if k == len(newSlice)-1 {
					newSlice = append(newSlice, val)
				}
			}
		}
	}
	return
}

//字符串->时间对象
func Str2Time(formatTimeStr string) time.Time {
	timeLayout := "2006-01-02 15:04:05"
	loc, _ := time.LoadLocation("Local")
	theTime, _ := time.ParseInLocation(timeLayout, formatTimeStr, loc) //使用模板在对应时区转化为time.time类型

	return theTime
}

func VerifyEmailFormat(email string) bool {
	pattern := `\w+([-+.]\w+)*@\w+([-.]\w+)*\.\w+([-.]\w+)*` //匹配电子邮箱
	reg := regexp.MustCompile(pattern)
	return reg.MatchString(email)
}

func VerifyIPFormat(ipstr string) bool {
	ip := net.ParseIP(ipstr)
	if ip == nil {
		return false
	}
	return true
}
