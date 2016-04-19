<?php

ini_set('display_errors','1');
error_reporting(E_ALL);

require "wpi.php";

$wpi = new WPI("localhost:9999", "/var/www/wp");
$wpi->setToken("abc");

$start = microtime(true);

function notify($event, $data = null) {
    echo "event: " . $event . PHP_EOL;
    if( $data ) {
        echo "data: " . json_encode($data) . PHP_EOL;
    }
    echo PHP_EOL;
    ob_flush();
    flush();
}

$wpi->start(function($info, $progress){
    notify("progress", array(
        'info'      => $info,
        'progress'  => $progress
    ));
});

notify("complete");

$time_elapsed_secs = microtime(true) - $start;

die("Imported in ". $time_elapsed_secs . "\n");