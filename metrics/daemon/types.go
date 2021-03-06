package daemon

import (
	"io/ioutil"
	"os/exec"

	"github.com/mperham/inspeqtor/metrics"
	"github.com/mperham/inspeqtor/util"
)

/*
 Daemon-specific metrics are process-specific metrics which can be pushed into Inspeqtor
 and used as rule checks.

 check service redis with hostname [localhost], port [6379], socket [nil], password [nil]
   if redis(latest_fork_usec) > 10000 then alert ops
*/

type collectorBuilder func(map[string]string) (Collector, error)
type metricMap map[string]float64
type executor func(string, []string, []byte) ([]byte, error)
type metric struct {
	name  string
	mtype metrics.Type
	funks *funcWrapper
}
type funcWrapper struct {
	disp  metrics.DisplayFunc
	xform metrics.TransformFunc
}

var (
	c                                   = metrics.Counter
	g                                   = metrics.Gauge
	Sources map[string]collectorBuilder = map[string]collectorBuilder{
		"redis":      buildRedisSource,
		"mysql":      buildMysqlSource,
		"memcached":  buildMemcachedSource,
		"nginx":      buildNginxSource,
		"postgresql": buildPostgresqlSource,
	}
	inMB = metrics.DisplayInMB
)

func NewStore(store metrics.Store, ds Collector) *Store {
	return &Store{store, ds}
}

type Store struct {
	metrics.Store
	DaemonSpecific Collector
}

func Prepare(ds *Store) error {
	return ds.DaemonSpecific.Prepare(execCmd)
}

func (ds *Store) Load(values ...interface{}) {
	ds.Store.(metrics.Loadable).Load(values...)
}

func (ds *Store) Watch(metricName string) {
	valid := ds.DaemonSpecific.ValidMetrics()
	for _, m := range valid {
		if m.name == metricName {
			dispFunk := metrics.DisplayFunc(nil)
			if m.funks != nil {
				dispFunk = m.funks.disp
			}

			if m.mtype == metrics.Counter {
				ds.Store.DeclareCounter(ds.DaemonSpecific.Name(), metricName, nil, dispFunk)
			} else {
				ds.Store.DeclareGauge(ds.DaemonSpecific.Name(), metricName, dispFunk)
			}
		}
	}
	ds.DaemonSpecific.Watch(metricName)
}

func (ds *Store) Collect(pid int) error {
	err := ds.Store.Collect(pid)
	if err != nil {
		return err
	}
	util.Debug("Collecting %s metrics", ds.DaemonSpecific.Name())
	hash, err := ds.DaemonSpecific.Capture()
	if err != nil {
		return err
	}
	for k, v := range hash {
		ds.Store.Save(ds.DaemonSpecific.Name(), k, v)
	}
	return nil
}

type Collector interface {
	Name() string
	// return a hash of metric:value pairs
	Capture() (metricMap, error)
	Prepare(executor) error
	Watch(metricName string)
	ValidMetrics() []metric
}

func execCmd(command string, args []string, stdin []byte) ([]byte, error) {
	cmd := exec.Command(command, args...)
	if stdin != nil {
		in, err := cmd.StdinPipe()
		if err != nil {
			return nil, err
		}
		_, err = in.Write(stdin)
		in.Close()
		if err != nil {
			return nil, err
		}
	}

	return util.SafeRun(cmd)
}

func testExec(path string) func(string, []string, []byte) ([]byte, error) {
	return func(command string, args []string, stdin []byte) ([]byte, error) {
		data, err := ioutil.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return data, nil
	}
}
