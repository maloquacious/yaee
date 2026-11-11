// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package sphere

import "math"

// TriangleArea returns the area of the spherical triangle with corners
// a, b and c on the unit sphere, in steradians: the area on a sphere of
// radius R is the result multiplied by R². The corners must have length
// 1. The result is not negative, whatever the order of the corners.
//
// It is computed with the formula of Van Oosterom and Strackee (1983),
// tan(Ω/2) = |a · (b × c)| / (1 + a·b + b·c + c·a), which stays accurate
// for the small triangles of a fine mesh.
func TriangleArea(a, b, c Vec) float64 {
	return 2 * math.Atan2(math.Abs(a.Dot(b.Cross(c))), 1+a.Dot(b)+b.Dot(c)+c.Dot(a))
}

// PointAreas returns the area of the sphere that each point of the Mesh
// stands for, in steradians: one third of the area of every triangle
// that has the point as a corner. The areas sum to 4π, the whole sphere,
// to within rounding.
//
// The triangles are added in order of triangle number, so the result is
// the same on every call.
func (m *Mesh) PointAreas() []float64 {
	areas := make([]float64, len(m.Points))
	for t := 0; t < len(m.Tri); t += 3 {
		a, b, c := m.Tri[t], m.Tri[t+1], m.Tri[t+2]
		third := TriangleArea(m.Points[a], m.Points[b], m.Points[c]) / 3
		areas[a] += third
		areas[b] += third
		areas[c] += third
	}
	return areas
}

// Distances returns, for every point of the Mesh, the geodesic angle in
// radians to the nearest of the source points, and which source that
// is. sources holds point numbers; a source is at distance 0 from
// itself. With no sources every distance is +Inf and every nearest
// source is −1.
//
// # Method and accuracy
//
// The nearest source is found by spreading outward from the sources
// along the edges of the Mesh: a point considers the sources that its
// neighbours have settled on, and takes the nearest of them. The
// distance returned is the exact angle between the point and the source
// it took, not a length measured along edges, so it has no error from
// the zig-zag of a path.
//
// The source taken is the true nearest source in all but rare cases,
// in which a nearer source was not offered by any neighbour. With
// scattered sources about 1 point in 3000 is affected, and its distance
// is too large by under a tenth of the spacing between points; with
// sources along a line, none was seen. TestDistances bounds both: at
// most 1 point in 1000, by at most half the spacing.
//
// The result depends only on the Mesh and on the set of sources, not on
// the order of sources.
//
// The cost is O(n log n) for n points.
func (m *Mesh) Distances(sources []int32) (dist []float64, nearest []int32) {
	n := len(m.Points)
	dist = make([]float64, n)
	nearest = make([]int32, n)
	for i := range dist {
		dist[i] = math.Inf(1)
		nearest[i] = -1
	}
	h := make(distHeap, 0, len(sources))
	for _, s := range sources {
		if dist[s] != 0 {
			dist[s], nearest[s] = 0, s
			h = append(h, distItem{0, s})
		}
	}
	// Every key is 0, so the slice is already a heap. Sources are
	// popped in order of point number, whatever order they came in.
	for len(h) > 0 {
		it := h.pop()
		if it.dist > dist[it.point] {
			continue // superseded by a nearer source
		}
		src := nearest[it.point]
		for _, j := range m.Neighbours(int(it.point)) {
			if nearest[j] == src {
				continue
			}
			d := Angle(m.Points[j], m.Points[src])
			if d < dist[j] || (d == dist[j] && src < nearest[j]) {
				dist[j], nearest[j] = d, src
				h.push(distItem{d, j})
			}
		}
	}
	return dist, nearest
}

// distItem is a point waiting to be spread from, with the distance it
// had when it was queued.
type distItem struct {
	dist  float64
	point int32
}

func (a distItem) less(b distItem) bool {
	return a.dist < b.dist || (a.dist == b.dist && a.point < b.point)
}

// distHeap is a binary min-heap of distItems.
type distHeap []distItem

func (h *distHeap) push(it distItem) {
	s := append(*h, it)
	i := len(s) - 1
	for i > 0 {
		parent := (i - 1) / 2
		if !s[i].less(s[parent]) {
			break
		}
		s[i], s[parent] = s[parent], s[i]
		i = parent
	}
	*h = s
}

func (h *distHeap) pop() distItem {
	s := *h
	top := s[0]
	last := len(s) - 1
	s[0] = s[last]
	s = s[:last]
	for i := 0; ; {
		l, r, least := 2*i+1, 2*i+2, i
		if l < last && s[l].less(s[least]) {
			least = l
		}
		if r < last && s[r].less(s[least]) {
			least = r
		}
		if least == i {
			break
		}
		s[i], s[least] = s[least], s[i]
		i = least
	}
	*h = s
	return top
}
