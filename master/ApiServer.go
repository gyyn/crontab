package master

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorhill/cronexpr"

	"github.com/gyyn/crontab/common"
)

//任务的http接口
type ApiServer struct {
	httpServer *http.Server
}

var (
	//单例对象
	G_apiServer *ApiServer
)

//判断任务配置参数是否合规
func handleJobJudge(resp http.ResponseWriter, req *http.Request) {
	var (
		err       error
		postJob   string
		job       common.Job
		bytes     []byte
		startTime time.Time
		stopTime  time.Time
		errno     int
	)

	//解析post表单
	if err = req.ParseForm(); err != nil {
		errno = -1
		goto ERR
	}

	//取表单中的job字段
	postJob = req.PostForm.Get("job")

	//反序列化job
	if err = json.Unmarshal([]byte(postJob), &job); err != nil {
		errno = -1
		goto ERR
	}

	//判断job的name
	if job.Name == "" {
		errno = -2
		err = errors.New("NameErr")
		goto ERR
	}
	//判断job的shell
	if job.Command == "" {
		errno = -3
		err = errors.New("CommandErr")
		goto ERR
	}

	//判断job的cron表达式
	if _, err = cronexpr.Parse(job.CronExpr); err != nil {
		errno = -4
		goto ERR
	}

	//判断job的报警email
	if job.Email == "" || !common.VerifyEmailFormat(job.Email) {
		errno = -5
		err = errors.New("EmailErr")
		goto ERR
	}

	//判断job的开始时间
	if job.StartTime != "" {
		startTime = common.Str2Time(job.StartTime)
		if startTime.IsZero() {
			errno = -6
			err = errors.New("StartTimeErr")
			goto ERR
		}
	}

	//判断job的停止时间
	if job.StopTime != "" {
		stopTime = common.Str2Time(job.StopTime)
		if stopTime.IsZero() {
			errno = -7
			err = errors.New("StopTimeErr")
			goto ERR
		}
	}

	//job开始时间在停止时间之后
	if !startTime.IsZero() && !stopTime.IsZero() && startTime.After(stopTime) {
		errno = -8
		err = errors.New("TimeErr")
		goto ERR
	}

	//判断job的详情
	if job.Details == "" {
		errno = -9
		err = errors.New("DetailsErr")
		goto ERR
	}

	//返回正常应答({"errno": 0, "msg": "", "data": {....}})
	if bytes, err = common.BuildResponse(0, "success", nil); err == nil {
		resp.Write(bytes)
	}
	return
ERR:
	//返回异常应答
	if bytes, err = common.BuildResponse(errno, err.Error(), nil); err == nil {
		resp.Write(bytes)
	}
}

//保存任务接口
//POST job={"name": "job1", "command": "echo hello", "cronExpr": "* * * * *"}
func handleJobSave(resp http.ResponseWriter, req *http.Request) {
	var (
		err     error
		postJob string
		job     common.Job
		oldJob  *common.Job
		bytes   []byte
	)

	//1.解析post表单
	if err = req.ParseForm(); err != nil {
		goto ERR
	}

	//2.取表单中的job字段
	postJob = req.PostForm.Get("job")

	//3.反序列化job
	if err = json.Unmarshal([]byte(postJob), &job); err != nil {
		goto ERR
	}

	//4.保存到etcd
	if oldJob, err = G_jobMgr.SaveJob(&job); err != nil {
		goto ERR
	}

	//5.返回正常应答({"errno": 0, "msg": "", "data": {....}})
	if bytes, err = common.BuildResponse(0, "success", oldJob); err == nil {
		resp.Write(bytes)
	}
	return
ERR:
	//6.返回异常应答
	if bytes, err = common.BuildResponse(-1, err.Error(), nil); err == nil {
		resp.Write(bytes)
	}
}

//删除任务接口
//POST /job/delete   name=job1
func handleJobDelete(resp http.ResponseWriter, req *http.Request) {
	var (
		err    error
		name   string
		oldJob *common.Job
		bytes  []byte
	)
	//POST:   a=1&b=2&c=3
	if err = req.ParseForm(); err != nil {
		goto ERR
	}

	//删除的任务名
	name = req.PostForm.Get("name")

	//去删除任务
	if oldJob, err = G_jobMgr.DeleteJob(name); err != nil {
		goto ERR
	}

	//正常应答
	if bytes, err = common.BuildResponse(0, "success", oldJob); err == nil {
		resp.Write(bytes)
	}
	return
ERR:
	if bytes, err = common.BuildResponse(-1, err.Error(), nil); err == nil {
		resp.Write(bytes)
	}
}

//列举所有crontab任务
func handleJobList(resp http.ResponseWriter, req *http.Request) {
	var (
		jobList []*common.Job
		bytes   []byte
		err     error
	)

	//获取任务列表
	if jobList, err = G_jobMgr.ListJobs(); err != nil {
		goto ERR
	}

	//正常应答
	if bytes, err = common.BuildResponse(0, "success", jobList); err == nil {
		resp.Write(bytes)
	}
	return
ERR:
	if bytes, err = common.BuildResponse(-1, err.Error(), nil); err == nil {
		resp.Write(bytes)
	}
}

//强制杀死某个任务
// POST /job/kill  name=job1
func handleJobKill(resp http.ResponseWriter, req *http.Request) {
	var (
		err   error
		name  string
		bytes []byte
	)

	//解析POST表单
	if err = req.ParseForm(); err != nil {
		goto ERR
	}

	//要杀死的任务名
	name = req.PostForm.Get("name")

	//杀死任务
	if err = G_jobMgr.KillJob(name); err != nil {
		goto ERR
	}

	//正常应答
	if bytes, err = common.BuildResponse(0, "success", nil); err == nil {
		resp.Write(bytes)
	}
	return

ERR:
	if bytes, err = common.BuildResponse(-1, err.Error(), nil); err == nil {
		resp.Write(bytes)
	}
}

//任务立即执行一次
//POST /job/once  name=job1
func handleJobOnce(resp http.ResponseWriter, req *http.Request) {
	var (
		err   error
		name  string
		bytes []byte
	)

	//解析POST表单
	if err = req.ParseForm(); err != nil {
		goto ERR
	}

	//要立即执行的任务名
	name = req.PostForm.Get("name")

	fmt.Println(name)

	//立即执行任务
	if err = G_jobMgr.OnceJob(name); err != nil {
		goto ERR
	}

	//正常应答
	if bytes, err = common.BuildResponse(0, "success", nil); err == nil {
		resp.Write(bytes)
	}
	return

ERR:
	if bytes, err = common.BuildResponse(-1, err.Error(), nil); err == nil {
		resp.Write(bytes)
	}
}

//查询任务日志
func handleJobLog(resp http.ResponseWriter, req *http.Request) {
	var (
		err        error
		name       string // 任务名字
		skipParam  string // 从第几条开始
		limitParam string // 返回多少条
		skip       int
		limit      int
		logArr     []*common.JobLog
		bytes      []byte
	)

	//解析GET参数
	if err = req.ParseForm(); err != nil {
		goto ERR
	}

	//获取请求参数 /job/log?name=job10&skip=0&limit=10
	name = req.Form.Get("name")
	skipParam = req.Form.Get("skip")
	limitParam = req.Form.Get("limit")
	if skip, err = strconv.Atoi(skipParam); err != nil {
		skip = 0
	}
	if limit, err = strconv.Atoi(limitParam); err != nil {
		limit = 20
	}

	if logArr, err = G_logMgr.ListLog(name, skip, limit); err != nil {
		goto ERR
	}

	//正常应答
	if bytes, err = common.BuildResponse(0, "success", logArr); err == nil {
		resp.Write(bytes)
	}
	return

ERR:
	if bytes, err = common.BuildResponse(-1, err.Error(), nil); err == nil {
		resp.Write(bytes)
	}
}

//查询任务最近工作节点
func handleJobRecentWorker(resp http.ResponseWriter, req *http.Request) {
	var (
		err        error
		name       string // 任务名字
		skipParam  string // 从第几条开始
		limitParam string // 返回多少条
		skip       int
		limit      int
		workerArr  []string
		bytes      []byte
	)

	//解析GET参数
	if err = req.ParseForm(); err != nil {
		goto ERR
	}

	//获取请求参数 /job/log?name=job10&skip=0&limit=10
	name = req.Form.Get("name")
	skipParam = req.Form.Get("skip")
	limitParam = req.Form.Get("limit")
	if skip, err = strconv.Atoi(skipParam); err != nil {
		skip = 0
	}
	if limit, err = strconv.Atoi(limitParam); err != nil {
		limit = 20
	}

	if workerArr, err = G_logMgr.ListRecentWorker(name, skip, limit); err != nil {
		goto ERR
	}

	//正常应答
	if bytes, err = common.BuildResponse(0, "success", workerArr); err == nil {
		resp.Write(bytes)
	}
	return

ERR:
	if bytes, err = common.BuildResponse(-1, err.Error(), nil); err == nil {
		resp.Write(bytes)
	}
}

//获取健康worker节点列表
func handleWorkerList(resp http.ResponseWriter, req *http.Request) {
	var (
		workerArr []string
		err       error
		bytes     []byte
	)

	if workerArr, err = G_workerMgr.ListWorkers(); err != nil {
		goto ERR
	}

	//正常应答
	if bytes, err = common.BuildResponse(0, "success", workerArr); err == nil {
		resp.Write(bytes)
	}
	return

ERR:
	if bytes, err = common.BuildResponse(-1, err.Error(), nil); err == nil {
		resp.Write(bytes)
	}
}

//POST worker={"user": "admin", "pwd": "hahaha", "addr": "111.111.111.111"}
func handleWorkerAdd(resp http.ResponseWriter, req *http.Request) {
	var (
		err           error
		postAddworker string
		workerSSH     common.WorkerSSH
		cli           SSHCli
		output        string
		bytes         []byte
	)

	//解析post表单
	if err = req.ParseForm(); err != nil {
		goto ERR
	}

	//取表单中的worker字段
	postAddworker = req.PostForm.Get("worker")

	//反序列化workerSSH
	if err = json.Unmarshal([]byte(postAddworker), &workerSSH); err != nil {
		goto ERR
	}

	cli = SSHCli{
		user: workerSSH.User,
		pwd:  workerSSH.Pwd,
		addr: workerSSH.Addr + ":22",
	}

	//if err = cli.SendFile("./worker", "./"); err != nil {
	//	goto ERR
	//}
	//if err = cli.SendFile("./worker.json", "./"); err != nil {
	//	goto ERR
	//}

	if output, err = cli.Run("chmod u+x ./worker"); err != nil {
		goto ERR
	}
	fmt.Printf("%v\n%v", output, err)

	if output, err = cli.Run("nohup ./worker -config ./worker.json > log.out 2>&1 &"); err != nil {
		goto ERR
	}
	fmt.Printf("%v\n%v", output, err)

	//返回正常应答
	if bytes, err = common.BuildResponse(0, "success", nil); err == nil {
		resp.Write(bytes)
	}
	return
ERR:
	//返回异常应答
	if bytes, err = common.BuildResponse(-1, err.Error(), nil); err == nil {
		resp.Write(bytes)
	}
}

func handleWorkerDelete(resp http.ResponseWriter, req *http.Request) {
	var (
		err           error
		postAddworker string
		workerSSH     common.WorkerSSH
		cli           SSHCli
		output        string
		psInfo        []string
		workerPid     string
		bytes         []byte
	)

	//解析post表单
	if err = req.ParseForm(); err != nil {
		goto ERR
	}

	//取表单中的worker字段
	postAddworker = req.PostForm.Get("worker")

	//反序列化workerSSH
	if err = json.Unmarshal([]byte(postAddworker), &workerSSH); err != nil {
		goto ERR
	}

	cli = SSHCli{
		user: workerSSH.User,
		pwd:  workerSSH.Pwd,
		addr: workerSSH.Addr + ":22",
	}

	if output, err = cli.Run("ps aux | grep ./worker.json | grep -v grep"); err != nil {
		goto ERR
	}
	fmt.Printf("%v\n%v", output, err)

	if output != "" {
		psInfo = strings.Fields(output)
		workerPid = psInfo[1]
	}
	fmt.Println(psInfo)
	fmt.Println(workerPid)

	if _, err = cli.Run("kill -9 " + workerPid); err != nil {
		goto ERR
	}

	//返回正常应答
	if bytes, err = common.BuildResponse(0, "success", nil); err == nil {
		resp.Write(bytes)
	}
	return
ERR:
	//返回异常应答
	if bytes, err = common.BuildResponse(-1, err.Error(), nil); err == nil {
		resp.Write(bytes)
	}
}

//判断Worker配置参数是否合规
func handleWorkerJudge(resp http.ResponseWriter, req *http.Request) {
	var (
		errno      int
		err        error
		postworker string
		workerSSH  common.WorkerSSH
		cli        SSHCli
		bytes      []byte
	)

	//解析post表单
	if err = req.ParseForm(); err != nil {
		errno = -1
		goto ERR
	}

	//取表单中的worker字段
	postworker = req.PostForm.Get("worker")

	//反序列化workerSSH
	if err = json.Unmarshal([]byte(postworker), &workerSSH); err != nil {
		errno = -1
		goto ERR
	}

	//判断worker用户名
	if workerSSH.User == "" {
		errno = -2
		err = errors.New("UserErr")
		goto ERR
	}

	//判断worker密码
	if workerSSH.Pwd == "" {
		errno = -3
		err = errors.New("PwdErr")
		goto ERR
	}

	//判断workerIP
	if workerSSH.Addr == "" || !common.VerifyIPFormat(workerSSH.Addr) {
		errno = -4
		err = errors.New("IPErr")
		goto ERR
	}

	//判断连接
	//cli = SSHCli{
	//	user: workerSSH.User,
	//	pwd:  workerSSH.Pwd,
	//	addr: workerSSH.Addr + ":22",
	//}

	//if _, err = cli.Connect(); err != nil {
	//	errno = -5
	//	goto ERR
	//}

	//返回正常应答
	if bytes, err = common.BuildResponse(0, "success", nil); err == nil {
		resp.Write(bytes)
	}
	return
ERR:
	//返回异常应答
	if bytes, err = common.BuildResponse(errno, err.Error(), nil); err == nil {
		resp.Write(bytes)
	}
}

//初始化服务
func InitApiServer() (err error) {
	var (
		mux           *http.ServeMux
		listener      net.Listener
		httpServer    *http.Server
		staticDir     http.Dir     //静态文件根目录
		staticHandler http.Handler //静态文件http回调
	)
	//配置路由
	mux = http.NewServeMux()
	mux.HandleFunc("/job/judge", handleJobJudge)
	mux.HandleFunc("/job/save", handleJobSave)
	mux.HandleFunc("/job/delete", handleJobDelete)
	mux.HandleFunc("/job/list", handleJobList)
	mux.HandleFunc("/job/kill", handleJobKill)
	mux.HandleFunc("/job/once", handleJobOnce)
	mux.HandleFunc("/job/log", handleJobLog)
	mux.HandleFunc("/job/recentworker", handleJobRecentWorker)
	mux.HandleFunc("/worker/list", handleWorkerList)
	mux.HandleFunc("/worker/add", handleWorkerAdd)
	mux.HandleFunc("/worker/delete", handleWorkerDelete)
	mux.HandleFunc("/worker/judge", handleWorkerJudge)

	///index.html
	//静态文件目录
	staticDir = http.Dir(G_config.WebRoot)
	staticHandler = http.FileServer(staticDir)
	mux.Handle("/", http.StripPrefix("/", staticHandler))
	//StripPrefix会去掉/index.html的/

	//启动tcp监听
	if listener, err = net.Listen("tcp", ":"+strconv.Itoa(G_config.ApiPort)); err != nil {
		return
	}

	//创建一个http服务
	httpServer = &http.Server{
		ReadTimeout:  time.Duration(G_config.ApiReadTimeout) * time.Millisecond,
		WriteTimeout: time.Duration(G_config.ApiWriteTimeout) * time.Millisecond,
		Handler:      mux,
	}

	//赋值单例
	G_apiServer = &ApiServer{
		httpServer: httpServer,
	}

	//启动服务端
	go httpServer.Serve(listener)

	return
}
