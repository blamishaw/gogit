package data_structures

import (
	"fmt"
	"math/rand"
)

type Set[T comparable] struct {
	elems map[T]bool
}

func NewSet[T comparable](items []T) Set[T] {
	m := make(map[T]bool)
	for _, item := range items {
		m[item] = true
	}
	return Set[T]{elems: m}
}

func (s *Set[T]) Add(items ...T) {
	for _, item := range items {
		s.elems[item] = true
	}
}

func (s *Set[T]) Length() int {
	return len(s.elems)
}

func (s *Set[T]) Includes(val T) bool {
	_, ok := s.elems[val]
	return ok
}

func (s *Set[T]) ToArray() []T {
	arr := make([]T, s.Length())

	i := 0
	for k := range s.elems {
		arr[i] = k
		i++
	}
	return arr
}

func (s *Set[T]) Pop() (T, error) {
	var res T
	if s.Length() == 0 {
		panic("set: Pop() called on empty set")
	}
	k := rand.Intn(s.Length())
	i := 0
	for elem := range s.elems {
		if i == k {
			delete(s.elems, elem)
			return elem, nil
		}
		i++
	}
	return res, fmt.Errorf("no item returned")
}

func (s Set[T]) String() string {
	return fmt.Sprintf("%#v", s.ToArray())
}
