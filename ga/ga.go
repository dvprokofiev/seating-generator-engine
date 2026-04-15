// Copyright (C) 2026 Прокофьев Даниил <d@dvprokofiev.ru>
// Лицензировано под GNU Affero General Public License v3.0
// Часть проекта генератора рассадок
package ga

import (
	"math"
	"math/rand"
	"runtime"
	"sync"
	"time"
)

type ClassConfig struct {
	Rows     int
	Columns  int
	deskType string
}

type Student struct {
	ID                      int
	Name                    string
	PreferredColumns        []int
	PreferredRows           []int
	MedicalPreferredColumns []int
	MedicalPreferredRows    []int
}

type optStudent struct {
	Student
	index int
	pCols map[int]bool
	pRows map[int]bool
	mCols map[int]bool
	mRows map[int]bool
}

type Request struct {
	Students        []Student
	Preferences     [][]int
	Forbidden       [][]int
	ClassConfig     ClassConfig
	PopSize         int
	Generations     int
	CrossOverChance float64
	PriorityWeights PriorityWeights
}

type SatisfactionDetails struct {
	Total      float64
	Medical    float64
	Friends    float64
	Enemies    float64
	Pref       float64
	RowBonus   float64
	Level      float64
	Complaints []string
}

type Response struct {
	SeatID       int
	Row          int
	Column       int
	Student      string
	StudentID    int
	Satisfaction SatisfactionDetails
}

type PriorityWeights struct {
	Medical     float64
	Preferences float64
	Friends     float64
	Enemies     float64
	Fill        float64
}

type Weights struct {
	RowBonus     float64
	MedPenalty   float64
	FriendBonus  float64
	EnemyPenalty float64
	PrefBonus    float64
}

type GAConfig struct {
	Generations     int
	PopSize         int
	CrossOverChance float64
}

func calcGAConfig(studentCount int) GAConfig {
	popSize := 300 + (studentCount * 20)
	if popSize > 2500 {
		popSize = 2500
	}

	return GAConfig{
		PopSize:         popSize,
		Generations:     5000,
		CrossOverChance: 0.8,
	}

}

func calculateWeights(pw PriorityWeights) Weights {
	return Weights{
		RowBonus:     float64(pw.Fill),
		PrefBonus:    float64(pw.Preferences),
		FriendBonus:  float64(pw.Friends),
		MedPenalty:   float64(pw.Medical),
		EnemyPenalty: float64(pw.Enemies),
	}
}

type SocialMap []bool

func abs(num int) int {
	if num < 0 {
		return -num
	}
	return num
}

func buildSocialMap(req Request, idToIndex map[int]int) (SocialMap, SocialMap, []int, []int) {
	n := len(req.Students)
	friends := make(SocialMap, n*n)
	enemies := make(SocialMap, n*n)
	friendsCount := make([]int, n)
	enemiesCount := make([]int, n)
	for _, pair := range req.Preferences {
		idx1, ok1 := idToIndex[pair[0]]
		idx2, ok2 := idToIndex[pair[1]]
		if ok1 && ok2 {
			friends[idx1*n+idx2] = true
			friends[idx2*n+idx1] = true
			friendsCount[idx1]++
			friendsCount[idx2]++
		}
	}
	for _, pair := range req.Forbidden {
		idx1, ok1 := idToIndex[pair[0]]
		idx2, ok2 := idToIndex[pair[1]]
		if ok1 && ok2 {
			enemies[idx1*n+idx2] = true
			enemies[idx2*n+idx1] = true
			enemiesCount[idx1]++
			enemiesCount[idx2]++
		}
	}
	return friends, enemies, friendsCount, enemiesCount
}

func scorePosition(row, totalRows int) float64 {
	if totalRows <= 1 {
		return 1.0
	}
	return 1.0 - (float64(row) / float64(totalRows-1))
}

func isSameDesk(col1, col2 int, seatType string) bool {
	seatsPerDesk := 2
	if seatType == "single" {
		seatsPerDesk = 1
	}
	return col1/seatsPerDesk == col2/seatsPerDesk
}

func decay(d int, lambda float64) float64 {
	return math.Exp(-lambda * float64(d))
}

func checkMed(student optStudent, row, col int, config ClassConfig) float64 {
	if len(student.mCols) == 0 && len(student.mRows) == 0 {
		return 0.0
	}

	score := 0.0
	count := 0.0

	if len(student.mCols) > 0 {
		minDist := config.Columns + 1
		for pc := range student.mCols {
			if d := abs(col - pc); d < minDist {
				minDist = d
			}
		}
		score += decay(minDist, 1.0)
		count++
	}

	if len(student.mRows) > 0 {
		minDist := config.Rows + 1
		for pc := range student.mRows {
			if d := abs(row - pc); d < minDist {
				minDist = d
			}
		}
		score += decay(minDist, 1.0)
		count++
	}

	return score / count
}

func checkPref(student optStudent, row, col int, config ClassConfig) float64 {
	if len(student.pCols) == 0 && len(student.pRows) == 0 {
		return 0.0
	}

	score := 0.0
	count := 0.0

	if len(student.pCols) > 0 {
		minDist := config.Columns + 1
		for pc := range student.pCols {
			if d := abs(col - pc); d < minDist {
				minDist = d
			}
		}
		score += decay(minDist, 0.4)
		count++
	}

	if len(student.pRows) > 0 {
		minDist := config.Rows + 1
		for pr := range student.pRows {
			if d := abs(row - pr); d < minDist {
				minDist = d
			}
		}
		score += decay(minDist, 0.4)
		count++
	}

	return score / count
}

func checkFriends(studentIdx int, seating []int, row, col int, config ClassConfig, friends SocialMap, n int, friendsCount []int) float64 {
	if friendsCount[studentIdx] == 0 {
		return 0.0
	}

	score := 0.0
	// Define local search radius to keep algorithm effective
	radius := 3

	for drow := -radius; drow <= radius; drow++ {
		for dcol := -radius; dcol <= radius; dcol++ {
			if drow == 0 && dcol == 0 {
				continue
			}

			nrow, ncol := row+drow, col+dcol

			if nrow >= 0 && nrow < config.Rows && ncol >= 0 && ncol < config.Columns {
				neighborIdx := seating[nrow*config.Columns+ncol]

				if neighborIdx >= 0 && neighborIdx < n && friends[studentIdx*n+neighborIdx] {
					dist := float64(abs(drow) + abs(dcol))

					if dist <= float64(radius) {
						contribution := 1.0 / dist
						// extra points if friends are sitting at the same desk
						if dist == 1 && drow == 0 && isSameDesk(col, ncol, config.deskType) {
							contribution *= 1.2
						}
						score += contribution
					}
				}
			}
		}
	}

	// normalize score
	finalScore := score / (float64(friendsCount[studentIdx]) * 1.2)
	if finalScore > 1.0 {
		return 1.0
	}
	return finalScore
}

func checkEnemies(studentIdx int, seating []int, row, col int, config ClassConfig, enemies SocialMap, n int, enemiesCount []int) float64 {
	if enemiesCount[studentIdx] == 0 {
		return 0.0
	}

	penalty := 0.0
	// shorter radius for enemies
	radius := 2

	for drow := -radius; drow <= radius; drow++ {
		for dcol := -radius; dcol <= radius; dcol++ {
			if drow == 0 && dcol == 0 {
				continue
			}

			nrow, ncol := row+drow, col+dcol

			if nrow >= 0 && nrow < config.Rows && ncol >= 0 && ncol < config.Columns {
				neighborIdx := seating[nrow*config.Columns+ncol]

				if neighborIdx >= 0 && neighborIdx < n && enemies[studentIdx*n+neighborIdx] {
					// use Manhattan distance
					dist := float64(abs(drow) + abs(dcol))

					if dist <= float64(radius) {
						// quadratic decay for enemies
						contribution := 1.0 / (dist * dist)

						// maximum penalty if sitting at the same desk
						if drow == 0 && isSameDesk(col, ncol, config.deskType) {
							contribution = 1.0
						}

						penalty += contribution
					}
				}
			}
		}
	}

	// normalize penalty to be in range [0, 1]
	finalPenalty := penalty / float64(enemiesCount[studentIdx])
	if finalPenalty > 1.0 {
		return 1.0
	}
	return finalPenalty
}

func fitness(seating []int, config ClassConfig, w Weights, friends SocialMap, enemies SocialMap, staticScores []float64, nStudents int, friendsCount []int, enemiesCount []int) float64 {
	score := 0.0
	sumWeights := w.FriendBonus + w.EnemyPenalty + w.MedPenalty + w.PrefBonus + w.RowBonus
	if sumWeights == 0 {
		sumWeights = 1
	}
	for i, studentIdx := range seating {
		if studentIdx < 0 || studentIdx >= nStudents {
			continue
		}
		row, col := i/config.Columns, i%config.Columns
		fScore := checkFriends(studentIdx, seating, row, col, config, friends, nStudents, friendsCount)
		ePenalty := checkEnemies(studentIdx, seating, row, col, config, enemies, nStudents, enemiesCount)

		sScore := (fScore * w.FriendBonus) - (ePenalty * w.EnemyPenalty)
		sScore += staticScores[studentIdx*config.Rows*config.Columns+i]

		sScore /= sumWeights

		score += sScore
	}
	return score / float64(nStudents)
}

func CrossOver(r *rand.Rand, parent1, parent2, child []int, used []bool) {
	N := len(parent1)
	for i := 0; i < N; i++ {
		used[i] = false
	}
	start, end := r.Intn(N), r.Intn(N)
	if start > end {
		start, end = end, start
	}
	for i := start; i <= end; i++ {
		child[i] = parent1[i]
		used[child[i]] = true
	}
	j := 0
	for i := 0; i < N; i++ {
		if i < start || i > end {
			for j < N && used[parent2[j]] {
				j++
			}
			if j < N {
				child[i] = parent2[j]
				used[child[i]] = true
				j++
			}
		}
	}
}

func localSearch(r *rand.Rand, seating []int, config ClassConfig, w Weights, friends, enemies SocialMap, staticScores []float64, nStudents int, friendsCount []int, enemiesCount []int, opt []optStudent) {
	N := len(seating)
	currentFit := fitness(seating, config, w, friends, enemies, staticScores, nStudents, friendsCount, enemiesCount)
	for i := 0; i < 30; i++ {
		idx1, idx2 := r.Intn(N), r.Intn(N)
		if idx1 == idx2 {
			continue
		}
		seating[idx1], seating[idx2] = seating[idx2], seating[idx1]
		newFit := fitness(seating, config, w, friends, enemies, staticScores, nStudents, friendsCount, enemiesCount)
		if newFit > currentFit {
			currentFit = newFit
		} else {
			seating[idx1], seating[idx2] = seating[idx2], seating[idx1]
		}
	}
}

func SwapMutation(r *rand.Rand, seating []int) {
	i1, i2 := r.Intn(len(seating)), r.Intn(len(seating))
	seating[i1], seating[i2] = seating[i2], seating[i1]
}

func tournamentSelection(r *rand.Rand, scores []float64, k int) int {
	bestIdx := r.Intn(len(scores))
	for i := 1; i < k; i++ {
		randIdx := r.Intn(len(scores))
		if scores[randIdx] > scores[bestIdx] {
			bestIdx = randIdx
		}
	}
	return bestIdx
}

func RunGA(req Request) ([]Response, float64, int) {
	N := req.ClassConfig.Columns * req.ClassConfig.Rows
	nStudents := len(req.Students)
	gaConfig := calcGAConfig(nStudents)
	popSize, generations := gaConfig.PopSize, gaConfig.Generations
	weights := calculateWeights(req.PriorityWeights)
	numCPU := runtime.NumCPU()

	idToIndex := make(map[int]int)
	opt := make([]optStudent, nStudents)
	for i, s := range req.Students {
		idToIndex[s.ID] = i
		m := func(sl []int) map[int]bool {
			r := make(map[int]bool)
			for _, v := range sl {
				r[v] = true
			}
			return r
		}
		opt[i] = optStudent{
			Student: s, index: i,
			pCols: m(s.PreferredColumns), pRows: m(s.PreferredRows),
			mCols: m(s.MedicalPreferredColumns), mRows: m(s.MedicalPreferredRows),
		}
	}

	staticScores := make([]float64, nStudents*N)
	for i := 0; i < nStudents; i++ {
		for seatIdx := 0; seatIdx < N; seatIdx++ {
			r, c := seatIdx/req.ClassConfig.Columns, seatIdx%req.ClassConfig.Columns
			mScore := checkMed(opt[i], r, c, req.ClassConfig)
			pScore := checkPref(opt[i], r, c, req.ClassConfig)
			rScore := scorePosition(r, req.ClassConfig.Rows)

			if mScore > 0 {
				mScore = weights.MedPenalty
			} else if mScore < 0 {
				mScore = -weights.MedPenalty
			}

			staticScores[i*N+seatIdx] = (pScore * weights.PrefBonus) + (rScore * weights.RowBonus) + mScore
		}
	}

	rands := make([]*rand.Rand, numCPU)
	for i := 0; i < numCPU; i++ {
		rands[i] = rand.New(rand.NewSource(time.Now().UnixNano() + int64(i)))
	}

	population := make([][]int, popSize)
	for i := range population {
		population[i] = rands[0].Perm(N)
	}
	friends, enemies, friendsCount, enemiesCount := buildSocialMap(req, idToIndex)

	newPop := make([][]int, popSize)
	for i := range newPop {
		newPop[i] = make([]int, N)
	}
	usedBufs := make([][]bool, popSize)
	for i := range usedBufs {
		usedBufs[i] = make([]bool, N)
	}

	scores := make([]float64, popSize)
	var wg sync.WaitGroup

	bestFitnessEver := -math.MaxFloat64
	stagnationCounter := 0
	stagnationLimit := 400 // Stop if no improvement for 400 generations
	totalGens := 1

	for gen := 0; gen < generations; gen++ {
		chunkSize := (popSize + numCPU - 1) / numCPU
		for w := 0; w < numCPU; w++ {
			start, end := w*chunkSize, (w+1)*chunkSize
			if start >= popSize {
				break
			}
			if end > popSize {
				end = popSize
			}
			wg.Add(1)
			go func(s, e int) {
				defer wg.Done()
				for i := s; i < e; i++ {
					scores[i] = fitness(population[i], req.ClassConfig, weights, friends, enemies, staticScores, nStudents, friendsCount, enemiesCount)
				}
			}(start, end)
		}
		wg.Wait()

		iBest := 0
		for i := 1; i < popSize; i++ {
			if scores[i] > scores[iBest] {
				iBest = i
			}
		}

		if scores[iBest] > bestFitnessEver {
			bestFitnessEver = scores[iBest]
		}

		if scores[iBest] > (bestFitnessEver + 0.0001) {
			bestFitnessEver = scores[iBest]
			stagnationCounter = 0
		} else {
			stagnationCounter++
		}

		if stagnationCounter >= stagnationLimit {
			// Early exit if results stopped improving
			break
		}

		// Adaptive Mutation Rate: Increase if we are stuck
		mutationRate := 0.15
		if stagnationCounter > 50 {
			mutationRate = 0.4 // "Shake" the population to escape local optima
		}

		copy(newPop[0], population[iBest])
		localSearch(rands[0], newPop[0], req.ClassConfig, weights, friends, enemies, staticScores, nStudents, friendsCount, enemiesCount, opt)

		for w := 0; w < numCPU; w++ {
			start, end := w*chunkSize, (w+1)*chunkSize
			if start == 0 {
				start = 1
			}
			if start >= popSize {
				break
			}
			if end > popSize {
				end = popSize
			}
			wg.Add(1)
			go func(s, e int, r *rand.Rand) {
				defer wg.Done()
				for i := s; i < e; i++ {
					p1Idx := tournamentSelection(r, scores, 3)
					p2Idx := tournamentSelection(r, scores, 3)
					CrossOver(r, population[p1Idx], population[p2Idx], newPop[i], usedBufs[i])
					if r.Float64() < mutationRate {
						SwapMutation(r, newPop[i])
					}
				}
			}(start, end, rands[w])
		}
		wg.Wait()

		population, newPop = newPop, population
		totalGens++
	}

	bestIdx := 0
	bestAns := fitness(population[0], req.ClassConfig, weights, friends, enemies, staticScores, nStudents, friendsCount, enemiesCount)
	for i := 1; i < popSize; i++ {
		Ans := fitness(population[i], req.ClassConfig, weights, friends, enemies, staticScores, nStudents, friendsCount, enemiesCount)
		if Ans > bestAns {
			bestAns = Ans
			bestIdx = i
		}
	}

	bestIndices := population[bestIdx]
	response := make([]Response, N)
	for i, studentIdx := range bestIndices {
		row, col := i/req.ClassConfig.Columns, i%req.ClassConfig.Columns
		if studentIdx >= nStudents || studentIdx < 0 {
			response[i] = Response{SeatID: i, Row: row, Column: col, Student: "-", StudentID: -1}
			continue
		}
		response[i] = Response{
			SeatID: i, Row: row, Column: col,
			Student: opt[studentIdx].Name, StudentID: opt[studentIdx].ID,
			Satisfaction: getSatisfactionDetails(bestIndices, row, col, studentIdx, weights, req.ClassConfig, friends, enemies, friendsCount, enemiesCount, opt),
		}
	}
	return response, bestAns, totalGens
}

func getSatisfactionDetails(seating []int, row, col, studentIndex int, w Weights, config ClassConfig, friends, enemies SocialMap, friendsCount []int, enemiesCount []int, students []optStudent) SatisfactionDetails {
	var details SatisfactionDetails
	student := students[studentIndex]

	mScore := checkMed(student, row, col, config)
	pScore := checkPref(student, row, col, config)
	fScore := checkFriends(studentIndex, seating, row, col, config, friends, len(students), friendsCount)
	ePenalty := checkEnemies(studentIndex, seating, row, col, config, enemies, len(students), enemiesCount)
	rScore := scorePosition(row, config.Rows)

	pMax := 0.0
	if len(student.mRows) > 0 || len(student.mCols) > 0 {
		pMax += w.MedPenalty
	}
	if len(student.pRows) > 0 || len(student.pCols) > 0 {
		pMax += w.PrefBonus
	}
	if friendsCount[studentIndex] > 0 {
		pMax += w.FriendBonus
	}

	if pMax == 0 {
		details.Total = 1.0

		details.Level = 0.9 + (0.1 * rScore)

		details.Medical = 0
		details.Pref = 0
		details.Friends = 0
		details.Enemies = 0
		details.RowBonus = rScore
	} else {
		details.Medical = (mScore * w.MedPenalty) / pMax
		if details.Medical < 0 {
			details.Medical = 0
		}

		details.Pref = (pScore * w.PrefBonus) / pMax
		details.Friends = (fScore * w.FriendBonus) / pMax

		details.Enemies = -(ePenalty * w.EnemyPenalty) / pMax

		details.Total = details.Medical + details.Pref + details.Friends + details.Enemies

		bonusImpact := 0.05
		details.RowBonus = rScore * bonusImpact

		details.Level = details.Total + details.RowBonus
	}

	if details.Level < 0 {
		details.Level = 0.0
	}
	if details.Level > 1.0 {
		details.Level = 1.0
	}

	if mScore < 0 {
		details.Complaints = append(details.Complaints, "Нарушены медицинские показания")
	}
	if ePenalty > 0 {
		details.Complaints = append(details.Complaints, "Рядом сидит нежелательный человек")
	}

	return details
}
