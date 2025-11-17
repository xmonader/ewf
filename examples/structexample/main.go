// Package main provides an example of storing and retrieving complex structs in workflow state.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/xmonader/ewf"
)

type Address struct {
	Street string `json:"street"`
	City   string `json:"city"`
}

type Person struct {
	Name    string  `json:"name"`
	Age     int     `json:"age"`
	Address Address `json:"address"`
}

func step1(ctx context.Context, state ewf.State) error {
	person := Person{
		Name: "Ahmed",
		Age:  301111,
		Address: Address{
			Street: "123 Main St",
			City:   "There",
		},
	}

	state["person"] = person

	log.Printf("Created and stored person: %+v", person)
	return nil
}

func step2(ctx context.Context, state ewf.State) error {
	personValue, exists := state["person"]
	if !exists {
		return fmt.Errorf("person not found in state")
	}

	person, ok := personValue.(Person)
	if !ok {
		return fmt.Errorf("failed to type assert person from state")
	}

	log.Printf("Retrieved person: Name=%s, Age=%d", person.Name, person.Age)
	log.Printf("Address: %s, %s", person.Address.Street, person.Address.City)

	person.Age++
	state["person"] = person
	log.Printf("Updated person age to: %d", person.Age)

	return nil
}

func main() {
	engine, err := ewf.NewEngine(nil)
	if err != nil {
		log.Fatalf("Failed to create engine: %v", err)
	}
	engine.Register("create_person", step1)
	engine.Register("access_person", step2)

	template := &ewf.WorkflowTemplate{
		Steps: []ewf.Step{
			{Name: "create_person"},
			{Name: "access_person"},
		},
	}
	engine.RegisterTemplate("struct-example", template)

	wf, err := engine.NewWorkflow("struct-example")
	if err != nil {
		log.Fatalf("Failed to create workflow: %v", err)
	}

	ctx := context.Background()
	if err := engine.Run(ctx, wf); err != nil {
		log.Fatalf("Workflow failed: %v", err)
	}

	log.Println("Workflow completed successfully!")

	finalPerson, ok := wf.State["person"].(Person)
	if ok {
		log.Printf("Final person state: %+v", finalPerson)
	}
}
