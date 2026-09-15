package com.admin.common.utils;

import com.admin.common.dto.GostDto;
import com.admin.entity.Forward;
import com.alibaba.fastjson.JSONObject;

/** Sends allow-listed host forwarder lifecycle commands to a node. */
public final class ForwarderUtil {
    private ForwarderUtil() {}

    public static GostDto apply(Long nodeId, Forward forward, String serviceName) {
        JSONObject data = request(forward);
        data.put("name", serviceName);
        data.put("listenPort", forward.getInPort());
        data.put("remote", forward.getRemoteAddr());
        return WebSocketServer.send_msg(nodeId, data, "ApplyForwarder");
    }

    public static GostDto pause(Long nodeId, Forward forward) {
        return WebSocketServer.send_msg(nodeId, request(forward), "PauseForwarder");
    }

    public static GostDto resume(Long nodeId, Forward forward) {
        return WebSocketServer.send_msg(nodeId, request(forward), "ResumeForwarder");
    }

    public static GostDto delete(Long nodeId, Forward forward) {
        return WebSocketServer.send_msg(nodeId, request(forward), "DeleteForwarder");
    }

    public static boolean isExternal(Forward forward) {
        return forward.getEngine() != null && !"gost".equalsIgnoreCase(forward.getEngine());
    }

    public static GostDto pauseAny(Long nodeId, Forward forward) {
        if ("realm".equalsIgnoreCase(forward.getEngine())) {
            return RealmUtil.pause(nodeId, forward.getId());
        }
        return pause(nodeId, forward);
    }

    public static GostDto deleteAny(Long nodeId, Forward forward) {
        if ("realm".equalsIgnoreCase(forward.getEngine())) {
            return RealmUtil.delete(nodeId, forward.getId());
        }
        return delete(nodeId, forward);
    }

    private static JSONObject request(Forward forward) {
        JSONObject data = new JSONObject();
        data.put("id", String.valueOf(forward.getId()));
        data.put("engine", forward.getEngine());
        return data;
    }
}
