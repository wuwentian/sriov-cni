package main

import (
"fmt"
"os"
"syscall"
)

// 定义一个 DirLock 的struct
type DirLock struct {
	dir string    // 目录路径，例如 /home/XXX/go/src
	f   *os.File  // 文件描述符
}

// 新建一个 DirLock
func New(dir string) *DirLock {
	return &DirLock{
		dir: dir,
	}
}

// 加锁操作
func (l *DirLock) Lock() error {
	f, err := os.Open(l.dir) // 获取文件描述符
	if err != nil {
		return err
	}
	l.f = f
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) // 加上排他锁，当遇到文件加锁的情况直接返回 Error
	if err != nil {
		return fmt.Errorf("cannot flock directory %s - %s", l.dir, err)
	}
	return nil
}

// 解锁操作
func (l *DirLock) Unlock() error {
	defer l.f.Close() // close 掉文件描述符
	return syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)} // 释放 Flock 文件锁