package app

import "testing"

func TestParseTestOptionsText(t *testing.T) {
	options, err := parseTestOptionsText(" first \nsecond\n\n third ")
	if err != nil {
		t.Fatalf("parse options: %v", err)
	}
	if len(options) != 3 {
		t.Fatalf("len(options)=%d", len(options))
	}
	if options[0].Key != "A" || options[0].Text != "first" || options[2].Key != "C" || options[2].Text != "third" {
		t.Fatalf("unexpected options: %#v", options)
	}
}

func TestParseTestCorrectText(t *testing.T) {
	options := []TestOption{{Key: "A", Text: "one"}, {Key: "B", Text: "two"}, {Key: "C", Text: "three"}}
	correct, err := parseTestCorrectText(" c, A ", options, "multiple", true)
	if err != nil {
		t.Fatalf("parse correct: %v", err)
	}
	if len(correct) != 2 || correct[0] != "A" || correct[1] != "C" {
		t.Fatalf("unexpected correct values: %#v", correct)
	}
	if _, err := parseTestCorrectText("A,C", options, "single", true); err == nil {
		t.Fatal("single-choice question accepted multiple correct answers")
	}
}

func TestSurveyQuestionIgnoresCorrectAnswers(t *testing.T) {
	options := []TestOption{{Key: "A", Text: "one"}, {Key: "B", Text: "two"}}
	correct, err := parseTestCorrectText("A", options, "survey_single", false)
	if err != nil {
		t.Fatalf("survey correct parse: %v", err)
	}
	if len(correct) != 0 {
		t.Fatalf("survey correct=%#v", correct)
	}
}
