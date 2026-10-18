package review

import (
	"strings"
	"testing"
)

func find(t *testing.T, ds []Draft, key string) Draft {
	t.Helper()
	for _, d := range ds {
		if d.Key == key {
			return d
		}
	}
	t.Fatalf("carte %q absente de %+v", key, ds)
	return Draft{}
}

func TestParseCloze(t *testing.T) {
	ds := Parse("Le {{c1::noyau}} gère les {{c2::processus::quoi ?}} et la {{c1::mémoire}}.")
	if len(ds) != 2 {
		t.Fatalf("2 cartes attendues, obtenu %d : %+v", len(ds), ds)
	}
	c1 := find(t, ds, "c1")
	if c1.Front != "Le […] gère les processus et la […]." {
		t.Errorf("recto c1 = %q", c1.Front)
	}
	if c1.Back != "Le ==noyau== gère les processus et la ==mémoire==." {
		t.Errorf("verso c1 = %q", c1.Back)
	}
	c2 := find(t, ds, "c2")
	if c2.Front != "Le noyau gère les [quoi ?] et la mémoire." {
		t.Errorf("recto c2 (indice) = %q", c2.Front)
	}
}

func TestParseIgnoreLeCode(t *testing.T) {
	src := "```go\nx := {{c1::map}}\n```\nUn `{{c2::bout}}` de code et `a :: b`.\n"
	if ds := Parse(src); len(ds) != 0 {
		t.Errorf("le code ne produit aucune carte, obtenu %+v", ds)
	}
}

func TestParseQuestionReponse(t *testing.T) {
	ds := Parse("- Qu'est-ce qu'un processus ? :: Un programme en cours d'exécution\nPas une carte : a::b\n")
	if len(ds) != 1 || ds[0].Kind != KindQA {
		t.Fatalf("1 carte QA attendue : %+v", ds)
	}
	if ds[0].Front != "Qu'est-ce qu'un processus ?" || ds[0].Back != "Un programme en cours d'exécution" {
		t.Errorf("QA = %+v", ds[0])
	}
}

func TestCleStable(t *testing.T) {
	a := Parse("Quelle est la capitale ? :: Paris")
	b := Parse("  quelle est  la CAPITALE ? :: Paris, ville lumière")
	if a[0].Key != b[0].Key {
		t.Errorf("la clé doit survivre aux retouches de forme : %s ≠ %s", a[0].Key, b[0].Key)
	}
	c := Parse("Le {{c1::noyau}} est central")
	d := Parse("Le {{c1::kernel}} est central, vraiment")
	if c[0].Key != d[0].Key {
		t.Error("modifier un trou ne doit pas changer sa clé")
	}
}

func TestParseTolerantEnCoursDeFrappe(t *testing.T) {
	for _, src := range []string{"{{c1::sans fin", "{{c::x}}", "{{cx::y}}", "{{c1::}}", "{{c123::x}}", "{c1::x}"} {
		if ds := Parse(src); len(ds) != 0 {
			t.Errorf("%q ne doit pas produire de carte : %+v", src, ds)
		}
	}
}

func TestParseMultiLignesEtDoublons(t *testing.T) {
	ds := Parse("- {{c1::TCP}} est fiable\n- {{c1::UDP}} est rapide\nQ ? :: R\nQ ? :: R\n")
	if len(ds) != 2 {
		t.Fatalf("1 trou c1 (2 lignes) + 1 QA dédoublonnée, obtenu %+v", ds)
	}
	c1 := find(t, ds, "c1")
	if strings.Count(c1.Front, "[…]") != 2 || !strings.Contains(c1.Front, "\n") {
		t.Errorf("les lignes d'un même trou sont réunies : %q", c1.Front)
	}
}

func TestNextClozeNumber(t *testing.T) {
	if n := NextClozeNumber("rien"); n != 1 {
		t.Errorf("premier numéro = %d", n)
	}
	if n := NextClozeNumber("{{c1::a}} {{c4::b}}"); n != 5 {
		t.Errorf("numéro suivant = %d, attendu 5", n)
	}
}
