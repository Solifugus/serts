package sim

import "strings"

// Names.
//
// People need names to be followed — a diary that says "character 147 married character
// 203" is a ledger, and one that says "Ваlen Torroth married Ysha Kelvane" is a story.
//
// Stored as a 4-byte seed, never as a string. §9.2 keeps Character free of pointers
// because Go's garbage collector skips large pointer-free arrays entirely, which is the
// most valuable optimisation available at population scale; a string field would put a
// pointer in every person and give that away. The name is generated from the seed when
// something needs to display it, which happens for a handful of people on a handful of
// frames, never in the tick loop.
//
// The names are not meant to be any real language's. They are syllables assembled under
// phonotactic rules — an onset, a nucleus, sometimes a coda — which is enough to make a
// word feel like it comes from somewhere: Fonek, Vitasha, Keldur, Amira.

var (
	// Onsets, weighted hard toward single consonants.
	//
	// The first table gave every cluster equal footing with every plain consonant and
	// produced Snuamindklurk, which is not a name. Real name-shapes are overwhelmingly
	// simple: fonek and vitasha are CV·CVC and CV·CV·CV, all single consonants. Clusters
	// are seasoning — present, rare, and never stacked against a coda.
	nameOnsets = []string{
		"b", "b", "d", "d", "f", "f", "g", "g", "h", "h", "k", "k", "k",
		"l", "l", "m", "m", "m", "n", "n", "n", "p", "p", "r", "r", "r",
		"s", "s", "s", "t", "t", "t", "v", "v", "z", "sh", "th", "ch",
		"br", "dr", "kr", "tr", "sl", "st",
	}
	// Vowel-initial names get their own small table rather than an empty onset, so Amira
	// and Oren are possible without vowels turning up mid-word where they would run
	// together with the syllable before.
	nameVowelStart = []string{"a", "e", "i", "o", "u"}
	// Nuclei: plain vowels overwhelmingly, a few diphthongs for colour.
	nameNuclei = []string{
		"a", "a", "a", "a", "a", "e", "e", "e", "e", "i", "i", "i", "i",
		"o", "o", "o", "o", "u", "u", "u", "ai", "ei", "ia", "ou",
	}
	// Codas: light, and mostly absent. A syllable ending on its vowel is what lets the
	// next one flow into it.
	nameCodas = []string{
		"", "", "", "", "", "", "l", "l", "n", "n", "n", "r", "r", "s", "s",
		"k", "k", "t", "t", "m", "d", "sh", "th",
	}
	// Family suffixes. Surnames want a different shape from given names or the two read
	// as interchangeable; a small set of endings does that cheaply.
	familySuffixes = []string{
		"", "", "en", "ar", "eth", "is", "orn", "ai", "ov", "esh", "ane", "ur",
	}
)

// nameRand is a tiny deterministic sequence, kept separate from the simulation's PRNG:
// generating a name for display must never disturb the stream the simulation draws from,
// or looking at somebody would change the future.
type nameRand uint32

func (r *nameRand) next(n int) int {
	// xorshift32 — short, stable, and good enough to pick syllables with.
	x := uint32(*r)
	x ^= x << 13
	x ^= x >> 17
	x ^= x << 5
	*r = nameRand(x)
	return int(x % uint32(n))
}

// GivenName builds a personal name from a seed: two syllables usually, three sometimes.
func GivenName(seed uint32) string {
	if seed == 0 {
		seed = 1 // xorshift is stuck at zero
	}
	r := nameRand(seed)
	syllables := 2
	if r.next(4) == 0 {
		syllables = 3
	}

	var b strings.Builder
	prevCoda := ""
	for i := 0; i < syllables; i++ {
		// One name in six begins on a vowel; the rest take a consonant.
		if i == 0 && r.next(6) == 0 {
			b.WriteString(nameVowelStart[r.next(len(nameVowelStart))])
		} else {
			onset := nameOnsets[r.next(len(nameOnsets))]
			// An onset repeating the coda before it stutters: Tat-tan, Kek-kor.
			if onset == prevCoda {
				onset = nameOnsets[r.next(len(nameOnsets))]
			}
			b.WriteString(onset)
			b.WriteString(nameNuclei[r.next(len(nameNuclei))])
		}
		coda := ""
		// Interior syllables rarely close, so consonants do not pile against the next
		// onset; the final one closes about half the time.
		if i == syllables-1 {
			coda = nameCodas[r.next(len(nameCodas))]
		} else if syllables == 2 && r.next(4) == 0 {
			// Interior codas only in short names. In a three-syllable name they stack —
			// Drot-man-gosh — and every one of them lands a consonant against the next
			// onset.
			coda = nameCodas[6+r.next(len(nameCodas)-6)]
		}
		b.WriteString(coda)
		prevCoda = coda
	}
	return title(b.String())
}

// FamilyName builds a family name from a seed. Longer than a given name and often
// suffixed, so a full name has two distinguishable halves.
func FamilyName(seed uint32) string {
	if seed == 0 {
		seed = 1
	}
	r := nameRand(seed ^ 0x9E3779B9) // a different stream from the given name
	var b strings.Builder
	prevCoda := ""
	for i := 0; i < 2; i++ {
		onset := nameOnsets[r.next(len(nameOnsets))]
		if onset == prevCoda {
			onset = nameOnsets[r.next(len(nameOnsets))]
		}
		b.WriteString(onset)
		b.WriteString(nameNuclei[r.next(len(nameNuclei))])
		prevCoda = ""
	}
	b.WriteString(familySuffixes[r.next(len(familySuffixes))])
	return title(b.String())
}

func title(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// Name is a character's given name.
func (s *State) Name(id CharID) string {
	if !s.AliveChar(id) && (id < 0 || int(id) >= len(s.Chars)) {
		return "?"
	}
	return GivenName(s.Chars[id].NameSeed)
}

// Family is a character's family name — inherited from the father where there is one, so
// a line can be followed down the generations and watched to thrive or die out.
func (s *State) Family(id CharID) string {
	if id < 0 || int(id) >= len(s.Chars) {
		return "?"
	}
	return FamilyName(s.Chars[id].FamilySeed)
}

// FullName is what a person is called.
func (s *State) FullName(id CharID) string {
	if id < 0 || int(id) >= len(s.Chars) {
		return "?"
	}
	return GivenName(s.Chars[id].NameSeed) + " " + FamilyName(s.Chars[id].FamilySeed)
}
