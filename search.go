package main

import (
	"fmt"
	"sort"
	"time"
)

type boundingKey [6]int64

type kdNode struct {
	row, axis, left, right int
}

// Search builds an exact six-dimensional L-infinity broad phase. Its counters
// describe the last Pairs iteration. Model geometry must remain unchanged while
// the index is in use; Pairs is sequential and is not safe for concurrent calls.
type Search struct {
	NInput, NExcluded, N, Candidates, Matches int
	BuildSeconds                              float64
	model                                     *Model
	tolerance                                 *Tolerance
	indices                                   []int
	features                                  []boundingKey
	radii                                     []int64
	nodes                                     []kdNode
	root                                      int
}

func NewSearch(model *Model, t *Tolerance, excludeSplays, includeDuplicates bool) *Search {
	start := time.Now()
	s := &Search{model: model, tolerance: t, NInput: len(model.Legs), root: -1}
	for i, leg := range model.Legs {
		excluded := false
		for _, flag := range leg.Flags {
			if flag == "SPLAY" && excludeSplays || flag == "DUPLICATE" && !includeDuplicates {
				excluded = true
				break
			}
		}
		if !excluded {
			s.indices = append(s.indices, i)
		}
	}
	s.N = len(s.indices)
	s.NExcluded = s.NInput - s.N
	s.features = make([]boundingKey, s.N)
	s.radii = make([]int64, s.N)
	rows := make([]int, s.N)
	for row, source := range s.indices {
		leg := model.Legs[source]
		for axis := 0; axis < 3; axis++ {
			low, high := leg.A[axis], leg.B[axis]
			if low > high {
				low, high = high, low
			}
			s.features[row][axis], s.features[row][axis+3] = low, high
		}
		s.radii[row] = QueryRadius(SquaredDistance(leg.A, leg.B), t)
		rows[row] = row
	}
	s.nodes = make([]kdNode, 0, s.N)
	s.root = s.buildTree(rows)
	s.BuildSeconds = time.Since(start).Seconds()
	return s
}

// Median partitioning keeps recursion bounded even for identical keys. This
// initial implementation sorts each partition using only the standard library;
// its build cost is O(n log^2 n), independent of output density.
func (s *Search) buildTree(rows []int) int {
	if len(rows) == 0 {
		return -1
	}
	low, high := s.features[rows[0]], s.features[rows[0]]
	for _, row := range rows[1:] {
		for axis, coordinate := range s.features[row] {
			if coordinate < low[axis] {
				low[axis] = coordinate
			}
			if coordinate > high[axis] {
				high[axis] = coordinate
			}
		}
	}
	axis := 0
	for k := 1; k < 6; k++ {
		if high[k]-low[k] > high[axis]-low[axis] {
			axis = k
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := s.features[rows[i]][axis], s.features[rows[j]][axis]
		if a == b {
			return rows[i] < rows[j]
		}
		return a < b
	})
	mid := len(rows) / 2
	index := len(s.nodes)
	s.nodes = append(s.nodes, kdNode{row: rows[mid], axis: axis, left: -1, right: -1})
	left, right := s.buildTree(rows[:mid]), s.buildTree(rows[mid+1:])
	s.nodes[index].left, s.nodes[index].right = left, right
	return index
}

func (s *Search) query(nodeIndex, first int, key boundingKey, radius int64, neighbors *[]int) {
	if nodeIndex < 0 {
		return
	}
	node := s.nodes[nodeIndex]
	feature := s.features[node.row]
	if node.row > first {
		inside := true
		for axis, coordinate := range feature {
			difference := coordinate - key[axis]
			if difference > radius || difference < -radius {
				inside = false
				break
			}
		}
		if inside {
			*neighbors = append(*neighbors, node.row)
		}
	}
	// Valid int32 coordinates plus the capped 2^32 radius fit in int64.
	if key[node.axis]-radius <= feature[node.axis] {
		s.query(node.left, first, key, radius, neighbors)
	}
	if key[node.axis]+radius >= feature[node.axis] {
		s.query(node.right, first, key, radius, neighbors)
	}
}

// Pairs emits complete ordered matches, retaining only current-batch neighbor
// lists. A dense query may have O(n) neighbors, and a batch O(batchSize*n).
func (s *Search) Pairs(batchSize int, emit func(Pair) error) error {
	if batchSize <= 0 {
		return fmt.Errorf("batch_size must be positive")
	}
	s.Candidates, s.Matches = 0, 0
	for first := 0; first < s.N; {
		count := batchSize
		if count > s.N-first {
			count = s.N - first
		}
		neighbors := make([][]int, count)
		for offset := range neighbors {
			row := first + offset
			s.query(s.root, row, s.features[row], s.radii[row], &neighbors[offset])
			sort.Ints(neighbors[offset])
		}
		for offset, row := range neighbors {
			sourceA := s.indices[first+offset]
			for _, compactB := range row {
				sourceB := s.indices[compactB]
				s.Candidates++
				if match := MatchLegs(s.model.Legs[sourceA], s.model.Legs[sourceB], s.tolerance); match != nil {
					s.Matches++
					if err := emit(Pair{A: sourceA, B: sourceB, Match: *match}); err != nil {
						return err
					}
				}
			}
		}
		first += count
	}
	return nil
}
