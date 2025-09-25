package main

func matchAspect(width, height int, aspect string) bool {
    switch aspect {
    case "2x3":
        return width*3 == height*2
    case "4x3":
        return width*3 == height*4
    case "16x9":
        return width*9 == height*16
    case "all", "":
        return true
    }
    return false
}
