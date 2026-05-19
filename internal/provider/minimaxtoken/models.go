package minimaxtoken

func WireModelName(model string) string {
	switch model {
	case "minimax-m2.7":
		return "MiniMax-M2.7"
	case "minimax-m2":
		return "MiniMax-M2"
	default:
		return model
	}
}
