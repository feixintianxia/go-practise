package paladin

import (
	"context"
	"errors"
	"flag"
)

var (
	DefaultClient Client
	confPath      string
)

func init() {
	flag.StringVar(&confPath, "conf", "", "default config path")
}

func Init(args ...interface{}) (err error) {
	if confPath != "" {
		DefaultClient, err = NewFile(confPath)
	} else {
		var (
			driver Driver
		)
		argsLackErr := errors.New("lack of remote config center args")
		if len(args) == 0 {
			panic(argsLackErr.Error())
		}

		argsInvalidErr := errors.New("invalid remote config center args")

		driverName, ok := args[0].(string)
		if !ok {
			panic(argsInvalidErr.Error())
		}

		driver, err = GetDriver(driverName)
		if err != nil {
			return
		}
		DefaultClient, err = driver.New()
	}

	if err != nil {
		return
	}
	return
}

func Watch(key string, s Setter) error {
	v := DefaultClient.Get(key)
	str, err := v.Raw()
	if err != nil {
		return err
	}

	if err := s.Set(str); err != nil {
		return err
	}

	go func() {
		for event := range WatchEvent(context.Background(), key) {
			s.Set(event.Value)
		}
	}()
	return nil
}

func WatchEvent(ctx context.Context, keys ...string) <-chan Event {
	return DefaultClient.WatchEvent(ctx, keys...)
}

func Get(key string) *Value {
	return DefaultClient.Get(key)
}

func GetAll() *Map {
	return DefaultClient.GetAll()
}

func keys() []string {
	return DefaultClient.GetAll().Keys()
}

func Close() error {
	return DefaultClient.Close()
}
