package datalayout

import (
	"math/rand"
	"testing"
)

// Array of Structs (AoS) - traditional approach
type Person struct {
	Name        string
	Age         int
	Placeholder [64]byte // padding to make struct larger
}

// Struct of Arrays (SoA) - cache-friendly approach
type PersonDatabase struct {
	Names        []string
	Ages         []int
	Placeholders [][64]byte
}

const defaultSize = 100_000

func newPersonArray(size int) []Person {
	rng := rand.New(rand.NewSource(42))
	persons := make([]Person, size)
	for i := range persons {
		persons[i] = Person{
			Name: "Person" + string(rune(i%26+'A')),
			Age:  rng.Intn(80) + 18, // age between 18 and 97
		}
	}
	return persons
}

func newPersonDatabase(size int) PersonDatabase {
	rng := rand.New(rand.NewSource(42))
	db := PersonDatabase{
		Names:        make([]string, size),
		Ages:         make([]int, size),
		Placeholders: make([][64]byte, size),
	}
	for i := range db.Ages {
		db.Names[i] = "Person" + string(rune(i%26+'A'))
		db.Ages[i] = rng.Intn(80) + 18
	}
	return db
}

func averageAgeAoS(persons []Person) float64 {
	if len(persons) == 0 {
		return 0
	}
	var sum int64
	for i := range persons {
		sum += int64(persons[i].Age)
	}
	return float64(sum) / float64(len(persons))
}

func averageAgeSoA(db PersonDatabase) float64 {
	if len(db.Ages) == 0 {
		return 0
	}
	var sum int64
	for i := range db.Ages {
		sum += int64(db.Ages[i])
	}
	return float64(sum) / float64(len(db.Ages))
}

func TestAverageEquality(t *testing.T) {
	persons := newPersonArray(1000)
	db := newPersonDatabase(1000)

	avgAoS := averageAgeAoS(persons)
	avgSoA := averageAgeSoA(db)

	if avgAoS != avgSoA {
		t.Errorf("averages don't match: AoS=%f, SoA=%f", avgAoS, avgSoA)
	}
}

// BenchmarkAverageAge_AoS tests average age calculation with Array of Structs
func BenchmarkAverageAge_AoS(b *testing.B) {
	persons := newPersonArray(defaultSize)
	var result float64
	for b.Loop() {
		result = averageAgeAoS(persons)
	}
	_ = result
}

// BenchmarkAverageAge_SoA tests average age calculation with Struct of Arrays
func BenchmarkAverageAge_SoA(b *testing.B) {
	db := newPersonDatabase(defaultSize)
	var result float64
	for b.Loop() {
		result = averageAgeSoA(db)
	}
	_ = result
}
