package recipe

// recipes is a list of each recipe.
var recipes []Recipe

// Register registers a new recipe.
func Register(recipe Recipe) {
	recipes = append(recipes, recipe)
}

// Recipes returns each recipe in a slice.
func Recipes() []Recipe {
	return recipes
}
