plugins {
    kotlin("jvm") version "2.1.20"
}

repositories {
    mavenCentral()
}

kotlin {
    jvmToolchain(21)
}

val junitVersion = "5.11.4"

dependencies {
    testImplementation(platform("org.junit:junit-bom:$junitVersion"))
    testImplementation("org.junit.jupiter:junit-jupiter")
    testImplementation("org.junit.jupiter:junit-jupiter-params")
    testRuntimeOnly("org.junit.platform:junit-platform-launcher")
}

tasks.test {
    useJUnitPlatform()
    ignoreFailures = true
}
