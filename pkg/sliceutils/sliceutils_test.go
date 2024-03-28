package sliceutils_test

import (
	"math/rand"
	"testing"
	"time"

	"github.com/Falokut/cinema_orders_service/pkg/sliceutils"
	"github.com/stretchr/testify/assert"
)

func TestUniqueMergeSlices_String(t *testing.T) {
	type args struct {
		base         []string
		guest        []string
		expectedBase []string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "string success test full merge",
			args: args{
				base:         []string{"a", "b"},
				guest:        []string{"c", "d"},
				expectedBase: []string{"a", "b", "c", "d"},
			},
		},
		{
			name: "string success test partial merge",
			args: args{
				base:         []string{"a", "b", "c"},
				guest:        []string{"c", "d"},
				expectedBase: []string{"a", "b", "c", "d"},
			},
		},
		{
			name: "string success test with empty guest",
			args: args{
				base:         []string{"a", "b", "c"},
				guest:        []string{},
				expectedBase: []string{"a", "b", "c"},
			},
		},
		{
			name: "string success test with empty base",
			args: args{
				base:         []string{},
				guest:        []string{"a", "b", "c"},
				expectedBase: []string{"a", "b", "c"},
			},
		},
		{
			name: "string success test with both empty",
			args: args{
				base:         []string{},
				guest:        []string{},
				expectedBase: []string{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sliceutils.UniqueMergeSlices(tt.args.base, tt.args.guest...)
			assert.Condition(t, func() (success bool) { return sliceutils.IsSlicesEqual(tt.args.expectedBase, got) })
		})
	}
}

var letters = []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func genRandomStringSlice(n int32) []string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	var res = make([]string, n)
	for i := range res {
		b := make([]byte, r.Intn(22))
		for i := range b {
			b[i] = letters[r.Intn(len(letters))]
		}
		res[i] = string(b)
	}
	return res
}
func BenchmarkUniqueMergeSlices_String100(b *testing.B) {
	s1 := genRandomStringSlice(50)
	s2 := genRandomStringSlice(50)
	b.StartTimer()
	sliceutils.UniqueMergeSlices(s1, s2...)
	b.StopTimer()
}
func BenchmarkUniqueMergeSlices_String1000(b *testing.B) {
	s1 := genRandomStringSlice(500)
	s2 := genRandomStringSlice(500)
	b.StartTimer()
	sliceutils.UniqueMergeSlices(s1, s2...)
	b.StopTimer()
}
func BenchmarkUniqueMergeSlices_String10000(b *testing.B) {
	s1 := genRandomStringSlice(5000)
	s2 := genRandomStringSlice(5000)
	b.StartTimer()
	sliceutils.UniqueMergeSlices(s1, s2...)
	b.StopTimer()
}
func TestUniqueMergeSlices_MyStruct(t *testing.T) {
	type S struct {
		S1 int
	}

	type args struct {
		base         []S
		guest        []S
		expectedBase []S
	}

	tests := []struct {
		name string
		args args
	}{
		{
			name: "int success test",
			args: args{
				base: []S{
					{
						S1: 1,
					},
					{
						S1: 2,
					},
				},
				guest: []S{
					{
						S1: 2,
					},
					{
						S1: 3,
					},
				},
				expectedBase: []S{
					{
						S1: 1,
					},
					{
						S1: 2,
					},
					{
						S1: 3,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sliceutils.UniqueMergeSlices(tt.args.base, tt.args.guest...)
			assert.Condition(t, func() (success bool) { return sliceutils.IsSlicesEqual(tt.args.expectedBase, got) })
		})
	}
}

func TestIsSlicesEqual_String(t *testing.T) {
	type args struct {
		s1       []string
		s2       []string
		expected bool
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "string success test",
			args: args{
				s1:       []string{"a", "b"},
				s2:       []string{"a", "b"},
				expected: true,
			},
		},
		{
			name: "string fail test",
			args: args{
				s1:       []string{"a", "b"},
				s2:       []string{"a", "f"},
				expected: false,
			},
		},
		{
			name: "string success test",
			args: args{
				s1:       []string{"a", "b", "c"},
				s2:       []string{"a", "b", "c"},
				expected: true,
			},
		},
		{
			name: "string fail test,len(s1)>len(s2)",
			args: args{
				s1:       []string{"a", "b", "d"},
				s2:       []string{"a", "b"},
				expected: false,
			},
		},
		{
			name: "string fail test,len(s2)>len(s1)",
			args: args{
				s1:       []string{"a", "b", "d"},
				s2:       []string{"a", "b", "d", "e", "f"},
				expected: false,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.args.expected, sliceutils.IsSlicesEqual(tt.args.s1, tt.args.s2), tt.name)
		})
	}
}
