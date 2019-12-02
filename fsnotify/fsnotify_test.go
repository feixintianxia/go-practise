package fsnotify
import (
    "fmt"
    "testing"
)

func TestWatcher(t *testing.T) {
    watcher := NewWatcher()
    w.Add("./1.txt")

    for {
        select {
        case <-w.Events:
            fmt.Println("file has change")
        }
    }

    w.Close()
}


