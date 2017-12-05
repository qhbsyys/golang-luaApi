//Lua.go

//Read the task configuration, and if there is an update, recreate the crontab
package luaApi

import (
	"fmt"
	"sort"

	"github.com/robfig/cron"
	"github.com/yuin/gopher-lua"
)

type RawJob struct {
	Sec, Min, Hour string
	LvmId          int
	FilePath       string
}

func (rj RawJob) Spec() string {
	return fmt.Sprintf("%s %s %s * * ?", rj.Sec, rj.Min, rj.Hour)
}

func (rj RawJob) Cmd(plf PreloadFunc) func() {
	if rj.LvmId == 0 {
		return func() {
			if err := DoFileOnce(rj.FilePath, plf); err != nil {
				panic(err) //will be catched by recover
			}
		}
	} else {
		return func() {
			if err := DoFileInLuaVM(rj.LvmId, rj.FilePath, plf); err != nil {
				panic(err) //will be catched by recover
			}
		}
	}
}

func (rj RawJob) Valid() error {
	var err error
	if rj.LvmId < 0 || rj.LvmId > MAX_LVM_NUM {
		err = fmt.Errorf("Invalid lvm num.")
	} else {
		_, err = cron.Parse(rj.Spec())
	}
	return err
}

type RawJobs []RawJob

func (s RawJobs) Len() int      { return len(s) }
func (s RawJobs) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s RawJobs) Less(i, j int) bool {
	return s[i].Spec() < s[j].Spec()
}

func loadCrontab(cronFile string) (RawJobs, error) {
	jobs := make([]RawJob, 0, 0)
	err := DoFileInLuaVM(1, cronFile, func(L *lua.LState) error {
		// func to get jobs from lua
		L.Register("setJobs", func(L2 *lua.LState) int {
			all := L2.CheckTable(1)
			for i := 0; i < all.MaxN(); i++ {
				tv := all.RawGetInt(i + 1).(*lua.LTable)
				job := RawJob{
					tv.RawGetString("sec").String(),
					tv.RawGetString("min").String(),
					tv.RawGetString("hour").String(),
					int(lua.LVAsNumber(tv.RawGetString("lvm"))),
					tv.RawGetString("doFile").String(),
				}
				if err := job.Valid(); err != nil {
					L2.RaiseError("Invalid job(idx %d: %v):%s", i+1, job, err)
					return 0
				}
				jobs = append(jobs, job)
			}
			return 0
		}) // Register func end
		return nil
	})
	return jobs, err
}

type Crontab struct {
	cur  RawJobs
	task *cron.Cron
}

//return need re-build-jobs
func (c *Crontab) needBuild(ld RawJobs) bool {
	sort.Sort(ld)

	if len(c.cur) != len(ld) {
		c.cur = ld
		return true
	} else {
		for i := range ld {
			if ld[i] != c.cur[i] {
				c.cur = ld
				return true
			}
		}
	}
	return false
}

func (c *Crontab) Load(cronFile string, plf PreloadFunc) error {
	ld, err := loadCrontab(cronFile)
	if err != nil {
		return err
	}

	if !c.needBuild(ld) {
		return nil
	}

	if c.task != nil {
		c.task.Stop()
		c.task = nil
	}
	c.task = cron.New()
	c.task.Start()

	for _, job := range c.cur {
		c.task.AddFunc(job.Spec(), job.Cmd(plf))
	}
	logger.Info("reload crontab for %d tasks", len(c.cur))
	return nil
}
